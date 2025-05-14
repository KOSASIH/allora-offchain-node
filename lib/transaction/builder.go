package transaction

import (
	"allora_offchain_node/lib/rpcclient"
	types "allora_offchain_node/lib/types"
	"context"
	"fmt"
	gomath "math"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/rs/zerolog/log"
)

func BuildAndSignTransaction(
	ctx context.Context,
	txParams *types.TransactionParams,
	encodingConfig moduletestutil.TestEncodingConfig,
	msgs ...sdktypes.Msg,
) ([]byte, error) {
	if err := txParams.Validate(); err != nil {
		return nil, err
	}
	log.Debug().Msgf("Building transaction with sequence %d", txParams.Sequence)
	// Create a new TxBuilder
	txBuilder := encodingConfig.TxConfig.NewTxBuilder()

	var memo string

	// Construct the message based on the message type
	var err error

	// Set the message and other transaction parameters
	if err := txBuilder.SetMsgs(msgs...); err != nil {
		return nil, err
	}
	// Gas calculation
	var gas uint64
	if txParams.GasEstimationConfig.OverrideGas > 0 {
		gas = txParams.GasEstimationConfig.OverrideGas
	} else {
		// Estimate gas limit
		totalTxSize := 0
		for _, msg := range msgs {
			totalTxSize += len(msg.String())
		}
		// Set gas limit
		gas, err = rpcclient.EstimateGas(totalTxSize, txParams.GasEstimationConfig)
		if err != nil {
			return nil, err
		}
	}
	// Apply adjustment safely
	if txParams.GasEstimationConfig.GasAdjustment > 0 {
		gasFloat := float64(gas) * txParams.GasEstimationConfig.GasAdjustment
		if gasFloat < gomath.MaxUint64 {
			gas = uint64(gasFloat)
		} else {
			gas = gomath.MaxUint64
		}
	}
	txBuilder.SetGasLimit(gas)
	// Calculate fees for tx, potentially override with a fixed value
	var fees math.Int
	if txParams.GasEstimationConfig.OverrideFees > 0 {
		// Set the gas price to the override value
		fees = math.NewIntFromUint64(txParams.GasEstimationConfig.OverrideFees)
	} else {
		// Calculate using gas limit and min gas price
		fees, err = rpcclient.CalculateFees(gas, txParams.GasEstimationConfig.MinGasPrice)
		if err != nil {
			return nil, err
		}
	}
	// Set fees for tx
	feeCoin := sdktypes.NewCoin(txParams.Denom, fees)
	txBuilder.SetFeeAmount(sdktypes.NewCoins(feeCoin))

	// Set fee granter (optional)
	if txParams.FeeGranterAddress != "" {
		granterAddr, err := sdktypes.AccAddressFromBech32(txParams.FeeGranterAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to parse fee granter address %v: %w", txParams.FeeGranterAddress, err)
		}
		txBuilder.SetFeeGranter(granterAddr)
	}

	// Set memo and timeout height
	txBuilder.SetMemo(memo)
	txBuilder.SetTimeoutHeight(txParams.TimeoutHeight)

	// Set up signature
	sigV2 := signing.SignatureV2{
		PubKey:   txParams.PubKey,
		Sequence: txParams.Sequence,
		Data: &signing.SingleSignatureData{ // nolint:exhaustruct
			SignMode: signing.SignMode_SIGN_MODE_DIRECT,
		},
	}

	if err := txBuilder.SetSignatures(sigV2); err != nil {
		return nil, err
	}

	signerData := authsigning.SignerData{ // nolint:exhaustruct
		ChainID:       txParams.ChainID,
		AccountNumber: txParams.AccNum,
		Sequence:      txParams.Sequence,
	}

	// Sign the transaction with the private key
	sigV2, err = tx.SignWithPrivKey(
		ctx,
		signing.SignMode_SIGN_MODE_DIRECT,
		signerData,
		txBuilder,
		txParams.PrivKey,
		encodingConfig.TxConfig,
		txParams.Sequence,
	)
	if err != nil {
		return nil, err
	}

	// Set the signed signature back to the txBuilder
	if err := txBuilder.SetSignatures(sigV2); err != nil {
		return nil, err
	}

	// Encode the transaction
	txBytes, err := encodingConfig.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, err
	}

	return txBytes, nil
}

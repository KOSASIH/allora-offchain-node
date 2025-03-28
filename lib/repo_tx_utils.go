package lib

import (
	"allora_offchain_node/lib/rpcclient"
	"allora_offchain_node/lib/transaction"
	types "allora_offchain_node/lib/types"
	"context"
	"errors"
	"fmt"
	"strings"

	errorsmod "cosmossdk.io/errors"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/rs/zerolog/log"
)

func (connectionManager *ConnectionManager) SendDataWithRetry(ctx context.Context, req sdktypes.Msg, infoMsg string, timeoutHeight uint64) (*coretypes.ResultBroadcastTx, error) {
	// Excess fees correction factor translated to fees using configured gas prices
	// This value is updated by the fee price update routine - making copy for consistency within method
	gasPrice := GetGasPrice()
	walletConfig, err := connectionManager.GetWalletConfig()
	if err != nil {
		return nil, err
	}
	wallet, err := connectionManager.GetWallet()
	if err != nil {
		return nil, err
	}
	// Transaction params are subject to change among the retries.
	txParams := &types.TransactionParams{
		ChainID:       walletConfig.ChainId,
		Denom:         DEFAULT_BOND_DENOM,
		Prefix:        ADDRESS_PREFIX,
		Sequence:      wallet.GetSequence(),
		AccNum:        wallet.GetAccountNumber(),
		PrivKey:       wallet.GetPrivKey(),
		PubKey:        wallet.GetPubKey(),
		TimeoutHeight: timeoutHeight,
		GasEstimationConfig: types.GasEstimationConfig{
			BaseGas:       walletConfig.BaseGas,
			GasPerByte:    walletConfig.GasPerByte,
			MinGasPrice:   gasPrice,
			SimulateTx:    walletConfig.SimulateGasFromStart,
			GasAdjustment: walletConfig.GasAdjustment,
			OverrideGas:   0,
			OverrideFees:  0,
		},
	}

	txNode, err := connectionManager.GetCurrentTxNode()
	if err != nil {
		log.Error().Err(err).Msg("failed to get current tx node, switching to next")
		txNode, err = connectionManager.SwitchToNextTxNode()
		if err != nil {
			return nil, fmt.Errorf("failed to switch tx node: %w", err)
		}
	}
	queryNode, err := connectionManager.GetCurrentQueryNode()
	if err != nil {
		log.Error().Err(err).Msg("failed to get current query node, switching to next")
		queryNode, err = connectionManager.SwitchToNextQueryNode()
		if err != nil {
			return nil, fmt.Errorf("failed to switch query node: %w", err)
		}
	}
	for retryCount := int64(0); retryCount <= walletConfig.MaxRetries; retryCount++ {
		log.Debug().Msgf("SendDataWithRetry iteration started (%d/%d)", retryCount, walletConfig.MaxRetries)
		txParams.Sequence = wallet.GetSequence()
		txResp, _, errTx := SendTransactionViaRPC(ctx, txNode.Chain.RPCClient, txNode.ServerAddress, queryNode, txParams, false, req)
		if errTx == nil {
			if txResp != nil {
				if strings.TrimSpace(txResp.Log) == "" {
					log.Info().Msgf("Transaction sent successfully: %v\n", txResp.Hash.String())
					wallet.IncrementSequence()
					return txResp, nil
				} else {
					log.Warn().Msgf("Transaction sent: %v, but nonempty error log: %s\n", txResp.Hash.String(), txResp.Log)
					// Creating error to process it below.
					errTx = fmt.Errorf("tx failed: %s", txResp.Log)
				}
			}

		}

		// Handle error on broadcasting
		errorResponse, err := ProcessErrorTx(ctx, errTx, infoMsg, retryCount, walletConfig.MaxRetries, txNode)
		switch errorResponse {
		case ErrorProcessingOk:
			return txResp, nil
		case ErrorProcessingError:
			// Error has not been handled, sleep and retry with regular delay
			if err != nil {
				log.Error().Err(err).Str("rpc", txNode.ServerAddress).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, walletConfig.MaxRetries)
				// Wait for the uniform delay before retrying
				if DoneOrWait(ctx, walletConfig.RetryDelay) {
					return nil, ctx.Err()
				}
				continue
			}
		case ErrorProcessingContinue:
			// Error has not been handled, just continue next iteration
			continue
		case ErrorProcessingResetSequence:
			txParams.Sequence = wallet.GetSequence()
			log.Warn().Msgf("Resetting sequence error on tx to current sequence %d", txParams.Sequence)
			continue
		case ErrorProcessingFees:
			// Error has not been handled, just mark as recalculate fees on this iteration
			log.Info().Msg("Insufficient fees, marking fee recalculation on tx broadcasting for retrial")
			// TODO Handle fee and "out of gas" error differently
			got, required, err := parseInsufficientFeeError(errTx.Error(), DEFAULT_BOND_DENOM)
			if err != nil {
				log.Error().Err(err).Msg("Failed to parse insufficient fee error")
			}
			log.Debug().Msgf("Retrying tx with required fee, got %d, required %d", got, required)
			if required > walletConfig.MaxFees.Number.BigInt().Uint64() {
				log.Error().Msgf("Required fee %d is greater than max fees %d", required, walletConfig.MaxFees)
				txParams.GasEstimationConfig.OverrideFees = walletConfig.MaxFees.Number.BigInt().Uint64()
			} else {
				txParams.GasEstimationConfig.OverrideFees = required
			}
			continue
		case ErrorProcessingGas:
			log.Info().Msg("Insufficient gas, marking gas recalculation on tx broadcasting for retrial")
			txParams.GasEstimationConfig.SimulateTx = true
			continue

		case ErrorProcessingFailure:
			return nil, errorsmod.Wrapf(err, "tx failed and not retried")
		case ErrorProcessingSwitchingNode:
			return nil, err
		default:
			return nil, errorsmod.Wrapf(err, "failed to process error")
		}
	}

	return nil, errorsmod.Wrapf(ErrUnexpectedError, "Tx failed after max retries")
}

// Sends transaction via RPC, and if configured to do so, simulates the tx gas calculation limit via GRPC query.
func SendTransactionViaRPC(ctx context.Context,
	rpcClient *rpcclient.AlloraRPCClient,
	rpcEndpoint string,
	queryNode *NodeConfig,
	txParams *types.TransactionParams,
	waitForTx bool,
	msgs ...sdktypes.Msg,
) (*coretypes.ResultBroadcastTx, string, error) {
	log.Debug().Msgf("Sending transaction via RPC to %s", rpcEndpoint)
	// Build and sign the transaction to get the bytes
	encodingConfig := transaction.GetEncodingConfig()
	txBytes, err := transaction.BuildAndSignTransaction(ctx, txParams, encodingConfig, msgs...)
	if err != nil {
		return nil, "", err
	}

	var gas uint64
	if txParams.GasEstimationConfig.SimulateTx {
		gas, _, err = simulateWithSequenceRetry(ctx,
			queryNode,
			txParams,
			encodingConfig,
			queryNode.ConnectionManager.walletConfig.MaxRetries,
			txBytes,
			msgs...)
		if err != nil {
			return nil, "", err
		}
		log.Debug().Msgf("Simulated gas: %d", gas)
		txParams.GasEstimationConfig.OverrideGas = gas
	}

	// Broadcast the transaction via RPC
	resp, err := rpcClient.BroadcastTx(ctx, txBytes, waitForTx)
	if err != nil {
		return resp, string(txBytes), fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	return resp, string(txBytes), nil
}

// Simulates the tx gas calculation limit via GRPC query.
// If the simulation fails due to account sequence mismatch, it resets the sequence and retries.
func simulateWithSequenceRetry(
	ctx context.Context,
	queryNode *NodeConfig,
	txParams *types.TransactionParams,
	encodingConfig testutil.TestEncodingConfig,
	maxRetries int64,
	initialTxBytes []byte,
	msgs ...sdktypes.Msg,
) (gas uint64, txBytes []byte, err error) {

	txBytes = initialTxBytes
	for retryCount := int64(0); retryCount <= maxRetries; retryCount++ {
		log.Debug().Msgf("Simulating tx with sequence retry %d/%d", retryCount, maxRetries)
		gas, err = queryNode.SimulateTxWithFallback(ctx, txBytes)
		if err == nil {
			return gas, txBytes, nil
		}

		if errors.Is(err, ErrTxSimulationError) {
			expected, current, err := parseSequenceFromAccountMismatchError(err.Error())
			if err != nil {
				return 0, nil, err
			}
			log.Warn().Msgf("Simulation error, resetting sequence to %d, was %d (retry %d/%d)",
				expected, current, retryCount, maxRetries)

			wallet, err := queryNode.ConnectionManager.GetWallet()
			if err != nil {
				return 0, nil, err
			}
			wallet.SetSequence(expected)
			txParams.Sequence = expected

			// Rebuild tx with new sequence
			log.Debug().Msgf("Rebuilding tx with new sequence %d", expected)
			txBytes, err = transaction.BuildAndSignTransaction(ctx, txParams, encodingConfig, msgs...)
			if err != nil {
				return 0, nil, err
			}
			continue
		}

		// For other errors, return immediately
		return 0, nil, err
	}

	return 0, nil, fmt.Errorf("tx simulation failed after %d retries", maxRetries)
}

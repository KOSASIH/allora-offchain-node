package transaction

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"

	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	types "allora_offchain_node/lib/types"

	cosmosmath "cosmossdk.io/math"
	"github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
)

func GeneratePrivKey() (cryptotypes.PrivKey, cryptotypes.PubKey, string) {
	algo := hd.Secp256k1

	privKey := secp256k1.GenPrivKey()
	privKeyCrypto := algo.Generate()(privKey.Bytes())
	pubKey := privKeyCrypto.PubKey()

	addressbytes := sdktypes.AccAddress(pubKey.Address().Bytes())
	address, err := sdktypes.Bech32ifyAddressBytes("allo", addressbytes)
	if err != nil {
		panic(err)
	}

	return privKeyCrypto, pubKey, address
}

func TestBuildAndSignTransactionWithDifferentParams(t *testing.T) {
	// Setup basic test environment
	ctx := context.Background()
	encodingConfig := GetEncodingConfig()
	// Register emissions types explicitly
	emissionstypes.RegisterInterfaces(encodingConfig.InterfaceRegistry)

	// Create test wallet
	privKey, pubKey, addr := GeneratePrivKey()

	testCases := []struct {
		name       string
		txParams   *types.TransactionParams
		sequence   uint64
		msgs       []sdktypes.Msg
		expectErr  bool
		errMessage string
	}{
		{
			name: "Basic transaction with minimum params",
			txParams: &types.TransactionParams{
				ChainID:       "test-chain",
				Denom:         "utest",
				Prefix:        "test",
				Sequence:      0,
				AccNum:        1,
				PrivKey:       privKey,
				PubKey:        pubKey,
				TimeoutHeight: 0,
				GasEstimationConfig: types.GasEstimationConfig{
					BaseGas:       2000,
					GasPerByte:    10,
					MinGasPrice:   0.1,
					GasAdjustment: 1.2,
					OverrideGas:   0,
					OverrideFees:  0,
					SimulateTx:    false,
				},
				FeeGranterAddress: "",
			},
			sequence: 0,
			msgs: []sdktypes.Msg{&emissionstypes.RegisterRequest{
				Sender:    addr,
				TopicId:   1,
				Owner:     addr,
				IsReputer: false,
			}},
			expectErr:  false,
			errMessage: "",
		},
		{
			name: "High gas configuration",
			txParams: &types.TransactionParams{
				ChainID:       "test-chain",
				Denom:         "utest",
				Prefix:        "test",
				Sequence:      0,
				AccNum:        1,
				PrivKey:       privKey,
				PubKey:        pubKey,
				TimeoutHeight: 100,
				GasEstimationConfig: types.GasEstimationConfig{
					BaseGas:       100000,
					GasPerByte:    1000,
					MinGasPrice:   1.0,
					GasAdjustment: 2.0,
					OverrideGas:   500000,
					OverrideFees:  0,
					SimulateTx:    false,
				},
				FeeGranterAddress: "",
			},
			sequence: 0,
			msgs: []sdktypes.Msg{&emissionstypes.RegisterRequest{
				Sender:    addr,
				TopicId:   1,
				Owner:     addr,
				IsReputer: false,
			}},
			expectErr:  false,
			errMessage: "",
		},
		{
			name: "Override fees configuration",
			txParams: &types.TransactionParams{
				ChainID:       "test-chain",
				Denom:         "utest",
				Prefix:        "test",
				Sequence:      0,
				AccNum:        1,
				PrivKey:       privKey,
				PubKey:        pubKey,
				TimeoutHeight: 0,
				GasEstimationConfig: types.GasEstimationConfig{
					BaseGas:       2000,
					GasPerByte:    10,
					MinGasPrice:   0.1,
					OverrideFees:  5000,
					OverrideGas:   0,
					SimulateTx:    false,
					GasAdjustment: 1.2,
				},
				FeeGranterAddress: "",
			},
			sequence: 0,
			msgs: []sdktypes.Msg{&emissionstypes.RegisterRequest{
				Sender:    addr,
				TopicId:   1,
				Owner:     addr,
				IsReputer: false,
			}},
			expectErr:  false,
			errMessage: "",
		},
		{
			name: "Multiple messages transaction",
			txParams: &types.TransactionParams{
				ChainID:       "test-chain",
				Denom:         "utest",
				Prefix:        "test",
				Sequence:      0,
				AccNum:        1,
				PrivKey:       privKey,
				PubKey:        pubKey,
				TimeoutHeight: 0,
				GasEstimationConfig: types.GasEstimationConfig{
					BaseGas:       2000,
					GasPerByte:    10,
					MinGasPrice:   0.1,
					GasAdjustment: 1.2,
					OverrideGas:   0,
					OverrideFees:  0,
					SimulateTx:    false,
				},
				FeeGranterAddress: "",
			},
			sequence: 0,
			msgs: []sdktypes.Msg{
				&emissionstypes.RegisterRequest{
					Sender:    addr,
					TopicId:   1,
					Owner:     addr,
					IsReputer: false,
				},
				&emissionstypes.RegisterRequest{
					Sender:    addr,
					TopicId:   2,
					Owner:     addr,
					IsReputer: false,
				},
			},
			expectErr:  false,
			errMessage: "",
		},
		{
			name: "Using a fee granter address",
			txParams: &types.TransactionParams{
				ChainID:       "test-chain",
				Denom:         "utest",
				Prefix:        "test",
				Sequence:      0,
				AccNum:        1,
				PrivKey:       privKey,
				PubKey:        pubKey,
				TimeoutHeight: 0,
				GasEstimationConfig: types.GasEstimationConfig{
					BaseGas:       2000,
					GasPerByte:    10,
					MinGasPrice:   0.1,
					GasAdjustment: 1.2,
					OverrideGas:   0,
					OverrideFees:  0,
					SimulateTx:    false,
				},
				FeeGranterAddress: "allo1urp932djsx64c0suy5r4w5f50teu43c3dgw5me",
			},
			sequence: 0,
			msgs: []sdktypes.Msg{&emissionstypes.RegisterRequest{
				Sender:    addr,
				TopicId:   1,
				Owner:     addr,
				IsReputer: false,
			}},
			expectErr:  false,
			errMessage: "",
		},
		{
			name: "Edge case - zero gas price",
			txParams: &types.TransactionParams{
				ChainID:       "test-chain",
				Denom:         "utest",
				Prefix:        "test",
				Sequence:      0,
				AccNum:        1,
				PrivKey:       privKey,
				PubKey:        pubKey,
				TimeoutHeight: 0,
				GasEstimationConfig: types.GasEstimationConfig{
					BaseGas:       2000,
					GasPerByte:    10,
					MinGasPrice:   0,
					GasAdjustment: 1.2,
					OverrideGas:   0,
					OverrideFees:  0,
					SimulateTx:    false,
				},
				FeeGranterAddress: "",
			},
			sequence: 0,
			msgs: []sdktypes.Msg{&emissionstypes.RegisterRequest{
				Sender:    addr,
				TopicId:   1,
				Owner:     addr,
				IsReputer: false,
			}},
			expectErr:  true,
			errMessage: "minimum gas price must be greater than zero",
		},
		{
			name: "Edge case - max uint64 gas",
			txParams: &types.TransactionParams{
				ChainID:       "test-chain",
				Denom:         "utest",
				Prefix:        "test",
				Sequence:      0,
				AccNum:        1,
				PrivKey:       privKey,
				PubKey:        pubKey,
				TimeoutHeight: 0,
				GasEstimationConfig: types.GasEstimationConfig{
					BaseGas:       math.MaxUint64,
					GasPerByte:    1,
					MinGasPrice:   0.1,
					GasAdjustment: 1.2,
					OverrideGas:   0,
					OverrideFees:  0,
					SimulateTx:    false,
				},
				FeeGranterAddress: "",
			},
			sequence: 0,
			msgs: []sdktypes.Msg{&emissionstypes.RegisterRequest{
				Sender:    addr,
				TopicId:   1,
				Owner:     addr,
				IsReputer: false,
			}},
			expectErr:  true,
			errMessage: "gas overflow",
		},
		{
			name: "Edge case - invalid fee granter address",
			txParams: &types.TransactionParams{
				ChainID:       "test-chain",
				Denom:         "utest",
				Prefix:        "test",
				Sequence:      0,
				AccNum:        1,
				PrivKey:       privKey,
				PubKey:        pubKey,
				TimeoutHeight: 0,
				GasEstimationConfig: types.GasEstimationConfig{
					BaseGas:       2000,
					GasPerByte:    10,
					MinGasPrice:   0.1,
					GasAdjustment: 1.2,
					OverrideGas:   0,
					OverrideFees:  0,
					SimulateTx:    false,
				},
				FeeGranterAddress: "muah1urp932djsx64c0suy5r4w5f50teu43c3dgw5me",
			},
			sequence: 0,
			msgs: []sdktypes.Msg{&emissionstypes.RegisterRequest{
				Sender:    addr,
				TopicId:   1,
				Owner:     addr,
				IsReputer: false,
			}},
			expectErr:  true,
			errMessage: "failed to parse fee granter address muah1urp932djsx64c0suy5r4w5f50teu43c3dgw5me: decoding bech32 failed: invalid checksum",
		},
	}

	config := sdktypes.GetConfig()
	config.SetBech32PrefixForAccount("allo", "allo")
	config.Seal()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build and sign transaction
			txBytes, err := BuildAndSignTransaction(
				ctx,
				tc.txParams,
				encodingConfig,
				tc.msgs...,
			)

			// Check results
			if tc.expectErr {
				require.Error(t, err)
				if tc.errMessage != "" {
					require.Contains(t, err.Error(), tc.errMessage)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, txBytes)

				// Decode transaction to verify parameters
				tx, err := encodingConfig.TxConfig.TxDecoder()(txBytes)
				require.NoError(t, err)

				// Verify transaction parameters
				authTx, ok := tx.(authsigning.Tx)
				require.True(t, ok)

				// Verify basic parameters
				require.Equal(t, tc.txParams.TimeoutHeight, authTx.GetTimeoutHeight())

				// Verify gas and fees with human-readable output
				actualGas := authTx.GetGas()
				if tc.txParams.GasEstimationConfig.OverrideGas > 0 {
					expectedGas := uint64(float64(tc.txParams.GasEstimationConfig.OverrideGas) * tc.txParams.GasEstimationConfig.GasAdjustment)
					require.Equal(t, expectedGas, actualGas,
						"Gas mismatch: expected %d, got %d", expectedGas, actualGas)
				} else {
					minBaseGas := tc.txParams.GasEstimationConfig.BaseGas
					require.GreaterOrEqual(t, actualGas, minBaseGas,
						"Gas mismatch: expected %d bigger than %d", actualGas, minBaseGas)
				}

				if tc.txParams.GasEstimationConfig.OverrideFees > 0 {
					overrideFees := tc.txParams.GasEstimationConfig.OverrideFees
					if overrideFees > uint64(math.MaxInt64) {
						t.Fatalf("OverrideFees exceeds MaxInt64")
					}
					expectedFee := sdktypes.NewCoin(tc.txParams.Denom,
						cosmosmath.NewInt(int64(overrideFees))) // nolint: gosec // reason: covered above
					require.Equal(t, sdktypes.NewCoins(expectedFee), authTx.GetFee())
				} else {
					minFee := sdktypes.NewCoin(tc.txParams.Denom,
						cosmosmath.NewInt(0))
					require.True(t, authTx.GetFee()[0].Amount.GT(minFee.Amount),
						"Fees mismatch: expected %d bigger than %d", authTx.GetFee()[0].Amount, minFee.Amount)
				}

				// Verify messages
				require.Equal(t, len(tc.msgs), len(tx.GetMsgs()))
				for i, msg := range tc.msgs {
					require.Equal(t, msg.String(), tx.GetMsgs()[i].String())
				}

				// Signatures
				signatures, err := authTx.GetSignaturesV2()
				require.NoError(t, err)
				require.Len(t, signatures, 1)
				require.Equal(t, tc.txParams.PrivKey.PubKey().Address().Bytes(), signatures[0].PubKey.Address().Bytes())
			}
		})
	}
}

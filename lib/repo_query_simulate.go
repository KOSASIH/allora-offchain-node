package lib

import (
	"context"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/types/query"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/rs/zerolog/log"
)

// SimulateTxWithRetry simulates a transaction with retry logic
func (node *NodeConfig) SimulateTxWithRetry(
	ctx context.Context,
	txBytes []byte,
) (uint64, error) {
	walletConfig, err := node.ConnectionManager.GetWalletConfig()
	if err != nil {
		return 0, errorsmod.Wrapf(err, "Error getting wallet config")
	}

	resp, err := QueryDataWithRetry(
		ctx,
		walletConfig.MaxRetries,
		walletConfig.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*txtypes.SimulateResponse, error) {
			simReq := &txtypes.SimulateRequest{ // nolint: exhaustruct // reason: deprecated use of Tx
				TxBytes: txBytes,
			}
			return node.Chain.TxServiceClient.Simulate(ctx, simReq)
		},
		query.PageRequest{}, // nolint:exhaustruct
		"simulate transaction",
		node,
	)
	if err != nil {
		return 0, err
	}

	if resp == nil || resp.GasInfo == nil {
		return 0, fmt.Errorf("simulation response or gas info is nil")
	}

	estimatedGas := resp.GasInfo.GasUsed
	log.Trace().
		Str("rpc", node.ServerAddress).
		Uint64("estimated_gas", estimatedGas).
		Msg("Retrieved gas estimation from chain")

	return estimatedGas, nil
}

// SimulateTxWithFallback attempts simulation across multiple nodes
func (node *NodeConfig) SimulateTxWithFallback(
	ctx context.Context,
	txBytes []byte,
) (uint64, error) {
	gas, err := node.SimulateTxWithRetry(ctx, txBytes)
	if err == nil {
		return gas, nil
	}

	// Try fallback nodes if primary fails
	log.Debug().
		Str("rpc", node.ServerAddress).
		Err(err).
		Msg("Primary node simulation failed, trying fallback nodes")

	// Get next available node
	nextNode, err := node.ConnectionManager.SwitchToNextQueryNode()
	if err != nil {
		return 0, errorsmod.Wrapf(err, "failed to switch to next node after simulation failure")
	}

	// Try simulation on new node
	return nextNode.SimulateTxWithRetry(ctx, txBytes)
}

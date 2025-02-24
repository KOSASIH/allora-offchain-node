package lib

import (
	"context"
	"fmt"
	"strconv"

	errorsmod "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/rs/zerolog/log"
	feemarkettypes "github.com/skip-mev/feemarket/x/feemarket/types"
)

// Keeps track of the current gas price
var gasPrice float64 = 0

// GetGasPrice returns the current gas price
func GetGasPrice() float64 {
	return gasPrice
}

// SetGasPrice sets the current gas price
func SetGasPrice(price float64) {
	gasPrice = price
}

// GetBaseFee queries the current base fee from the feemarket module
func (node *NodeConfig) GetBaseFee(ctx context.Context, denom string) (float64, error) {
	walletConfig, err := node.ConnectionManager.GetWalletConfig()
	if err != nil {
		return 0, errorsmod.Wrapf(err, "Error getting wallet config")
	}
	resp, err := QueryDataWithRetry(
		ctx,
		walletConfig.MaxRetries,
		walletConfig.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*feemarkettypes.GasPriceResponse, error) {
			return node.Chain.FeeMarketQueryClient.GasPrice(ctx, &feemarkettypes.GasPriceRequest{Denom: denom})
		},
		query.PageRequest{}, // nolint:exhaustruct
		"get base fee",
		node,
	)
	if err != nil {
		return 0, err
	}

	// Convert legacyDec to string first, then to float64
	baseFeeStr := resp.Price.Amount.String()
	baseFee, err := strconv.ParseFloat(baseFeeStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse base fee: %w", err)
	}

	log.Trace().Str("rpc", node.ServerAddress).Float64("baseFee", baseFee).Msg("Retrieved base fee from chain")
	return baseFee, nil
}

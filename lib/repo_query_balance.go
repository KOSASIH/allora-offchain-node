package lib

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	cosmossdk_io_math "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func (node *NodeConfig) GetBalance(ctx context.Context, inAddress string, denom string) (cosmossdk_io_math.Int, error) {
	walletConfig, err := node.ConnectionManager.GetWalletConfig()
	if err != nil {
		return cosmossdk_io_math.Int{}, errorsmod.Wrapf(err, "Error getting wallet config")
	}
	resp, err := QueryDataWithRetry(
		ctx,
		walletConfig.MaxRetries,
		walletConfig.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*banktypes.QueryBalanceResponse, error) {
			return node.Chain.BankQueryClient.Balance(ctx, &banktypes.QueryBalanceRequest{
				Address: inAddress,
				Denom:   denom,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"get balance",
		node,
	)
	if err != nil {
		return cosmossdk_io_math.Int{}, err
	}

	return resp.Balance.Amount, nil
}

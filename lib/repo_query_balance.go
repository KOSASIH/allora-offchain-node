package lib

import (
	"context"

	cosmossdk_io_math "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func (node *NodeConfig) GetBalance(ctx context.Context) (cosmossdk_io_math.Int, error) {
	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		node.Wallet.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*banktypes.QueryBalanceResponse, error) {
			return node.Chain.BankQueryClient.Balance(ctx, &banktypes.QueryBalanceRequest{
				Address: node.Chain.Address,
				Denom:   node.Chain.DefaultBondDenom,
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

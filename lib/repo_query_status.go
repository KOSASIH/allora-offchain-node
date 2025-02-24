package lib

import (
	"context"

	cmtservice "github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	"github.com/cosmos/cosmos-sdk/types/query"
)

func (node *NodeConfig) GetBlockHeight(ctx context.Context, walletConfig *WalletConfig) (BlockHeight, error) {
	resp, err := QueryDataWithRetry(
		ctx,
		walletConfig.MaxRetries,
		walletConfig.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*cmtservice.GetLatestBlockResponse, error) {
			return node.Chain.CometQueryClient.GetLatestBlock(ctx, &cmtservice.GetLatestBlockRequest{})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"get block height",
		node,
	)
	if err != nil {
		return 0, err
	}

	return resp.SdkBlock.Header.Height, nil
}

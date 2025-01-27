package lib

import (
	"context"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

func (node *NodeConfig) GetReputerValuesAtBlock(ctx context.Context, topicId emissionstypes.TopicId, nonce BlockHeight) (*emissionstypes.ValueBundle, error) {
	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		node.Wallet.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.GetNetworkInferencesAtBlockResponse, error) {
			return node.Chain.EmissionsQueryClient.GetNetworkInferencesAtBlock(ctx, &emissionstypes.GetNetworkInferencesAtBlockRequest{
				TopicId:                  topicId,
				BlockHeightLastInference: nonce,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"get reputer values at block",
		node,
	)
	if err != nil {
		return &emissionstypes.ValueBundle{}, err
	}

	return resp.NetworkInferences, nil
}

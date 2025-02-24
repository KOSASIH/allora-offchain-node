package lib

import (
	"context"
	"errors"

	errorsmod "cosmossdk.io/errors"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// Gets topic info for a given topic ID, with retries
func (node *NodeConfig) GetTopicInfo(ctx context.Context, topicId emissionstypes.TopicId) (*emissionstypes.Topic, error) {
	walletConfig, err := node.ConnectionManager.GetWalletConfig()
	if err != nil {
		return nil, errorsmod.Wrapf(err, "Error getting wallet config")
	}
	resp, err := QueryDataWithRetry(
		ctx,
		walletConfig.MaxRetries,
		walletConfig.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.GetTopicResponse, error) {
			return node.Chain.EmissionsQueryClient.GetTopic(ctx, &emissionstypes.GetTopicRequest{
				TopicId: topicId,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"get topic info",
		node,
	)
	if err != nil {
		return nil, err
	}

	if resp.Topic == nil {
		return nil, errors.New("Topic not found")
	}
	return resp.Topic, nil
}

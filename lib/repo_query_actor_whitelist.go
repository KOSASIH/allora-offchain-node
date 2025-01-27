package lib

import (
	"context"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// Checks if a worker can submit to a given topic
func (node *NodeConfig) CanSubmitWorker(ctx context.Context, topicId emissionstypes.TopicId, address string) (bool, error) {
	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		node.Wallet.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.CanSubmitWorkerPayloadResponse, error) {
			return node.Chain.EmissionsQueryClient.CanSubmitWorkerPayload(ctx, &emissionstypes.CanSubmitWorkerPayloadRequest{
				TopicId: topicId,
				Address: address,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"check worker whitelist",
		node,
	)
	if err != nil {
		return false, err
	}

	return resp.CanSubmitWorkerPayload, nil
}

// Checks if a reputer can submit to a given topic
func (node *NodeConfig) CanSubmitReputer(ctx context.Context, topicId emissionstypes.TopicId, address string) (bool, error) {

	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		node.Wallet.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.CanSubmitReputerPayloadResponse, error) {
			return node.Chain.EmissionsQueryClient.CanSubmitReputerPayload(ctx, &emissionstypes.CanSubmitReputerPayloadRequest{
				TopicId: topicId,
				Address: address,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"check reputer whitelist",
		node,
	)
	if err != nil {
		return false, err
	}

	return resp.CanSubmitReputerPayload, nil
}

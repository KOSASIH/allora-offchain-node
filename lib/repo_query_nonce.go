package lib

import (
	"context"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// Gets the latest open worker nonce for a given topic, with retries
func (node *NodeConfig) GetLatestOpenWorkerNonceByTopicId(ctx context.Context, topicId emissionstypes.TopicId) (*emissionstypes.Nonce, error) {
	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		node.Wallet.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.GetUnfulfilledWorkerNoncesResponse, error) {
			return node.Chain.EmissionsQueryClient.GetUnfulfilledWorkerNonces(ctx, &emissionstypes.GetUnfulfilledWorkerNoncesRequest{
				TopicId: topicId,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"get open worker nonce",
		node,
	)
	if err != nil {
		return &emissionstypes.Nonce{}, err // nolint: exhaustruct
	}

	if len(resp.Nonces.Nonces) == 0 {
		return &emissionstypes.Nonce{}, nil // nolint: exhaustruct
	}
	// Per `AddWorkerNonce()` in `allora-chain/x/emissions/keeper.go`, the latest nonce is first
	return resp.Nonces.Nonces[0], nil
}

// Gets the oldest open reputer nonce for a given topic, with retries
func (node *NodeConfig) GetOldestReputerNonceByTopicId(ctx context.Context, topicId emissionstypes.TopicId) (*emissionstypes.Nonce, error) {
	resp, err := QueryDataWithRetry(
		ctx,
		node.Wallet.MaxRetries,
		node.Wallet.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.GetUnfulfilledReputerNoncesResponse, error) {
			return node.Chain.EmissionsQueryClient.GetUnfulfilledReputerNonces(ctx, &emissionstypes.GetUnfulfilledReputerNoncesRequest{
				TopicId: topicId,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"get open reputer nonce",
		node,
	)
	if err != nil {
		return &emissionstypes.Nonce{}, err // nolint: exhaustruct
	}

	if len(resp.Nonces.Nonces) == 0 {
		return &emissionstypes.Nonce{}, nil // nolint: exhaustruct
	}
	// Per `AddWorkerNonce()` in `allora-chain/x/emissions/keeper.go`, the oldest nonce is last
	return resp.Nonces.Nonces[len(resp.Nonces.Nonces)-1].ReputerNonce, nil
}

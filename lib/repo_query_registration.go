package lib

import (
	"context"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

// Checks if the worker is registered in a topic, with retries
func (node *NodeConfig) IsWorkerRegistered(ctx context.Context, topicId uint64) (bool, error) {
	walletConfig, err := node.ConnectionManager.GetWalletConfig()
	if err != nil {
		return false, err
	}

	wallet, err := node.ConnectionManager.GetWallet()
	if err != nil {
		return false, err
	}

	resp, err := QueryDataWithRetry(
		ctx,
		walletConfig.MaxRetries,
		walletConfig.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.IsWorkerRegisteredInTopicIdResponse, error) {
			return node.Chain.EmissionsQueryClient.IsWorkerRegisteredInTopicId(ctx, &emissionstypes.IsWorkerRegisteredInTopicIdRequest{
				TopicId: topicId,
				Address: wallet.Address,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"is worker registered in topic",
		node,
	)
	if err != nil {
		return false, err
	}

	return resp.IsRegistered, nil
}

// Checks if the reputer is registered in a topic, with retries
func (node *NodeConfig) IsReputerRegistered(ctx context.Context, topicId uint64) (bool, error) {
	walletConfig, err := node.ConnectionManager.GetWalletConfig()
	if err != nil {
		return false, err
	}

	wallet, err := node.ConnectionManager.GetWallet()
	if err != nil {
		return false, err
	}

	resp, err := QueryDataWithRetry(
		ctx,
		walletConfig.MaxRetries,
		walletConfig.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*emissionstypes.IsReputerRegisteredInTopicIdResponse, error) {
			return node.Chain.EmissionsQueryClient.IsReputerRegisteredInTopicId(ctx, &emissionstypes.IsReputerRegisteredInTopicIdRequest{
				TopicId: topicId,
				Address: wallet.Address,
			})
		},
		query.PageRequest{}, // nolint: exhaustruct
		"is reputer registered in topic",
		node,
	)
	if err != nil {
		return false, err
	}

	return resp.IsRegistered, nil
}

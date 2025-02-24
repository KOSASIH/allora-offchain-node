package usecase

import (
	"allora_offchain_node/lib"
	"allora_offchain_node/metrics"
	"context"
)

type UseCaseSuite struct {
	UserConfig        lib.UserConfig
	ConnectionManager lib.ConnectionManagerInterface
	Metrics           *metrics.Metrics
}

// Static method to create a new UseCaseSuite
func NewUseCaseSuite(ctx context.Context, userConfig lib.UserConfig, connectionManager lib.ConnectionManagerInterface) (*UseCaseSuite, error) {
	err := userConfig.ValidateConfigAdapters()
	if err != nil {
		return nil, err
	}

	return &UseCaseSuite{UserConfig: userConfig, ConnectionManager: connectionManager}, nil // nolint: exhaustruct
}

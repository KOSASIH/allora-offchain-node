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
	essentialCtx      context.Context
	nonEssentialCtx   context.Context
}

// Static method to create a new UseCaseSuite
func NewUseCaseSuite(
	essentialCtx context.Context,
	nonEssentialCtx context.Context,
	Metrics *metrics.Metrics,
	userConfig lib.UserConfig,
	connectionManager lib.ConnectionManagerInterface) (*UseCaseSuite, error) {
	err := userConfig.ValidateConfigAdapters()
	if err != nil {
		return nil, err
	}

	return &UseCaseSuite{
		UserConfig:        userConfig,
		ConnectionManager: connectionManager,
		Metrics:           Metrics,
		essentialCtx:      essentialCtx,
		nonEssentialCtx:   nonEssentialCtx,
	}, nil
}

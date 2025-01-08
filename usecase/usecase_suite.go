package usecase

import (
	lib "allora_offchain_node/lib"
)

type UseCaseSuite struct {
	Node    lib.NodeConfig
	Metrics lib.Metrics
}

// Static method to create a new UseCaseSuite
func NewUseCaseSuite(userConfig lib.UserConfig) (*UseCaseSuite, error) {
	err := userConfig.ValidateConfigAdapters()
	if err != nil {
		return nil, err
	}
	userConfig.CheckAndSetDefaults()
	nodeConfig, err := userConfig.GenerateNodeConfig()
	if err != nil {
		return nil, err
	}

	return &UseCaseSuite{Node: *nodeConfig}, nil // nolint: exhaustruct
}

package usecase

import (
	lib "allora_offchain_node/lib"
)

type UseCaseSuite struct {
	UserConfig lib.UserConfig
	RPCManager RPCManagerInterface
	Metrics    lib.Metrics
}

// Static method to create a new UseCaseSuite
func NewUseCaseSuite(userConfig lib.UserConfig) (*UseCaseSuite, error) {
	err := userConfig.ValidateConfigAdapters()
	if err != nil {
		return nil, err
	}

	rpcManager, err := NewRPCManager(userConfig)
	if err != nil {
		return nil, err
	}
	return &UseCaseSuite{UserConfig: userConfig, RPCManager: rpcManager}, nil // nolint: exhaustruct
}

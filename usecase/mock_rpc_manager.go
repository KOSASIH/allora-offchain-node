package usecase

import (
	"allora_offchain_node/lib"
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/stretchr/testify/mock"
)

type MockRPCManager struct {
	mock.Mock
}

func (m *MockRPCManager) GetCurrentNode() *lib.NodeConfig {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	if node, ok := args.Get(0).(*lib.NodeConfig); ok {
		return node
	}
	return nil
}

func (m *MockRPCManager) SwitchToNextNode() *lib.NodeConfig {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	if node, ok := args.Get(0).(*lib.NodeConfig); ok {
		return node
	}
	return nil
}

func (m *MockRPCManager) GetStats() (int, map[int]int) {
	args := m.Called()
	failures, ok := args.Get(1).(map[int]int)
	if !ok {
		return args.Int(0), make(map[int]int)
	}
	return args.Int(0), failures
}

func (m *MockRPCManager) GetNodes() ([]lib.NodeConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if nodes, ok := args.Get(0).([]lib.NodeConfig); ok {
		return nodes, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRPCManager) SendDataWithNodeRetry(ctx context.Context, msg sdk.Msg, timeoutHeight uint64, operationName string) (*cosmosclient.Response, error) {
	args := m.Called(ctx, msg, timeoutHeight, operationName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if response, ok := args.Get(0).(*cosmosclient.Response); ok {
		return response, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRPCManager) GetCurrentIndex() int {
	args := m.Called()
	return args.Int(0)
}

func (m *MockRPCManager) SwitchToNode(index int) *lib.NodeConfig {
	args := m.Called(index)
	if args.Get(0) == nil {
		return nil
	}
	if node, ok := args.Get(0).(*lib.NodeConfig); ok {
		return node
	}
	return nil
}

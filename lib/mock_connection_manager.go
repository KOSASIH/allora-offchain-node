package lib

import (
	"context"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/mock"
)

type MockConnectionManager struct {
	mock.Mock
}

func (m *MockConnectionManager) GetCurrentQueryNode() (*NodeConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if node, ok := args.Get(0).(*NodeConfig); ok {
		return node, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) GetCurrentTxNode() (*NodeConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if node, ok := args.Get(0).(*NodeConfig); ok {
		return node, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) SwitchToNextQueryNode() (*NodeConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if node, ok := args.Get(0).(*NodeConfig); ok {
		return node, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) SwitchToNextTxNode() (*NodeConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if node, ok := args.Get(0).(*NodeConfig); ok {
		return node, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) GetStats() (int, map[int]int) {
	args := m.Called()
	failures, ok := args.Get(1).(map[int]int)
	if !ok {
		return args.Int(0), make(map[int]int)
	}
	return args.Int(0), failures
}

func (m *MockConnectionManager) GetQueryNodes() ([]NodeConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if nodes, ok := args.Get(0).([]NodeConfig); ok {
		return nodes, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) GetTxNodes() ([]NodeConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if nodes, ok := args.Get(0).([]NodeConfig); ok {
		return nodes, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) SendDataWithNodeRetry(ctx context.Context, msg sdk.Msg, timeoutHeight uint64, operationName string) (*coretypes.ResultBroadcastTx, error) {
	args := m.Called(ctx, msg, timeoutHeight, operationName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if response, ok := args.Get(0).(*coretypes.ResultBroadcastTx); ok {
		return response, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) SendDataWithRetry(ctx context.Context, req sdk.Msg, infoMsg string, timeoutHeight uint64) (*coretypes.ResultBroadcastTx, error) {
	args := m.Called(ctx, req, infoMsg, timeoutHeight)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if response, ok := args.Get(0).(*coretypes.ResultBroadcastTx); ok {
		return response, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) GetCurrentQueryIndex() int {
	args := m.Called()
	return args.Int(0)
}

func (m *MockConnectionManager) GetCurrentTxIndex() int {
	args := m.Called()
	return args.Int(0)
}

func (m *MockConnectionManager) SwitchToQueryNode(index int) (*NodeConfig, error) {
	args := m.Called(index)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if node, ok := args.Get(0).(*NodeConfig); ok {
		return node, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) SwitchToTxNode(index int) (*NodeConfig, error) {
	args := m.Called(index)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if node, ok := args.Get(0).(*NodeConfig); ok {
		return node, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockConnectionManager) GetWallet() (*Wallet, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if wallet, ok := args.Get(0).(*Wallet); ok {
		return wallet, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConnectionManager) GetWalletConfig() (*WalletConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if walletConfig, ok := args.Get(0).(*WalletConfig); ok {
		return walletConfig, args.Error(1)
	}
	return nil, args.Error(1)
}

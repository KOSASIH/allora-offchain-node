package lib

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"

	errorsmod "cosmossdk.io/errors"
	"github.com/rs/zerolog/log"

	metrics "allora_offchain_node/metrics"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type ConnectionManagerInterface interface {
	GetCurrentQueryNode() (*NodeConfig, error)
	GetCurrentTxNode() (*NodeConfig, error)
	GetCurrentQueryIndex() int
	GetCurrentTxIndex() int
	SwitchToNextQueryNode() (*NodeConfig, error)
	SwitchToNextTxNode() (*NodeConfig, error)
	SwitchToQueryNode(index int) (*NodeConfig, error)
	SwitchToTxNode(index int) (*NodeConfig, error)
	SendDataWithNodeRetry(ctx context.Context, msg sdk.Msg, timeoutHeight uint64, operationName string) (*coretypes.ResultBroadcastTx, error)
	SendDataWithRetry(ctx context.Context, req sdk.Msg, infoMsg string, timeoutHeight uint64) (*coretypes.ResultBroadcastTx, error)
	GetQueryNodes() ([]NodeConfig, error)
	GetTxNodes() ([]NodeConfig, error)
	GetWallet() (*Wallet, error)
	GetWalletConfig() (*WalletConfig, error)
	Close() error
}

const (
	GRPC_MODE int = 1
	RPC_MODE  int = 2
)

type ConnectionManager struct {
	queryNodes   []NodeConfig
	txNodes      []NodeConfig
	queryIdx     int
	txIdx        int
	queryMu      sync.RWMutex
	txMu         sync.RWMutex
	wallet       *Wallet
	walletConfig *WalletConfig
}

// Creates a new ConnectionManager instance, initialising the wallet and the connection nodes
func NewConnectionManager(ctx context.Context, userConfig UserConfig) (*ConnectionManager, error) {
	if len(userConfig.Wallet.NodeRPCs) == 0 {
		return nil, fmt.Errorf("no RPC nodes provided")
	}
	if len(userConfig.Wallet.NodeGRPCs) == 0 {
		return nil, fmt.Errorf("no GRPC nodes provided")
	}

	wallet, err := NewWalletFromConfig(ctx, userConfig.Wallet)
	if err != nil {
		return nil, err
	}

	// validate all urls are correct
	err = validateNodeURIs(userConfig.Wallet.NodeRPCs)
	if err != nil {
		return nil, err
	}

	// validate all urls are correct
	err = validateNodeURIs(userConfig.Wallet.NodeGRPCs)
	if err != nil {
		return nil, err
	}

	// Create a new ConnectionManager partially initialized with the wallet and wallet config,
	// it will be completed later
	var connectionManager = &ConnectionManager{ // nolint:exhaustruct
		wallet:       wallet,
		walletConfig: &userConfig.Wallet,
	}

	// Load here the nodeconfigs
	var queryNodes []NodeConfig
	for _, endpoint := range userConfig.Wallet.NodeGRPCs {
		log.Info().Str("grpc", endpoint).Msg("Initializing grpc query nodes")
		nodeConfig, err := userConfig.GenerateNodeConfig(ctx, wallet, GRPC_MODE, endpoint)
		if err != nil {
			log.Error().Err(err).Str("grpc", endpoint).Msg("Error generating node config, skipping GRPC node")
			continue
		}
		nodeConfig.ConnectionManager = connectionManager
		queryNodes = append(queryNodes, *nodeConfig)
	}
	if len(queryNodes) == 0 {
		return nil, fmt.Errorf("no query nodes initialized")
	}

	var txNodes []NodeConfig

	for _, endpoint := range userConfig.Wallet.NodeRPCs {
		log.Info().Str("rpc", endpoint).Msg("Initializing rpc tx nodes")
		nodeConfig, err := userConfig.GenerateNodeConfig(ctx, wallet, RPC_MODE, endpoint)
		if err != nil {
			log.Error().Err(err).Str("rpc", endpoint).Msg("Error generating node config, skipping RPC node")
			continue
		}
		nodeConfig.ConnectionManager = connectionManager
		txNodes = append(txNodes, *nodeConfig)
	}
	if len(txNodes) == 0 {
		return nil, fmt.Errorf("no tx nodes initialized")
	}

	connectionManager.queryNodes = queryNodes
	connectionManager.txNodes = txNodes
	connectionManager.queryIdx = 0
	connectionManager.txIdx = 0

	// Initialize the wallet with the account info
	node, err := connectionManager.GetCurrentQueryNode()
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}
	_, sequence, accNum, err := node.GetAccountInfo(ctx, wallet.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}
	wallet.SetSequence(sequence)
	wallet.SetAccountNumber(accNum)
	log.Info().Msgf("Wallet initialized successfully, with account (sequence: %d, accNum: %d)", sequence, accNum)

	return connectionManager, nil
}

// GetWallet returns the wallet instance, returns error if wallet is not initialized
func (connectionManager *ConnectionManager) GetWallet() (*Wallet, error) {
	if connectionManager.wallet == nil {
		return nil, fmt.Errorf("wallet not initialized")
	}
	return connectionManager.wallet, nil
}

func (connectionManager *ConnectionManager) GetWalletConfig() (*WalletConfig, error) {
	if connectionManager.walletConfig == nil {
		return nil, fmt.Errorf("wallet config not initialized")
	}
	return connectionManager.walletConfig, nil
}

func (connectionManager *ConnectionManager) GetQueryNodes() ([]NodeConfig, error) {
	connectionManager.queryMu.RLock()
	defer connectionManager.queryMu.RUnlock()
	return connectionManager.queryNodes, nil
}

func (connectionManager *ConnectionManager) GetTxNodes() ([]NodeConfig, error) {
	connectionManager.txMu.RLock()
	defer connectionManager.txMu.RUnlock()
	return connectionManager.txNodes, nil
}

func (connectionManager *ConnectionManager) GetCurrentQueryIndex() int {
	connectionManager.queryMu.RLock()
	defer connectionManager.queryMu.RUnlock()
	return connectionManager.queryIdx
}

func (connectionManager *ConnectionManager) GetCurrentTxIndex() int {
	connectionManager.txMu.RLock()
	defer connectionManager.txMu.RUnlock()
	return connectionManager.txIdx
}

func (connectionManager *ConnectionManager) GetCurrentQueryNode() (*NodeConfig, error) {
	connectionManager.queryMu.RLock()
	defer connectionManager.queryMu.RUnlock()
	if connectionManager.queryIdx < 0 || connectionManager.queryIdx >= len(connectionManager.queryNodes) {
		return nil, fmt.Errorf("invalid query index, not returning node")
	}
	return &connectionManager.queryNodes[connectionManager.queryIdx], nil
}

func (connectionManager *ConnectionManager) GetCurrentTxNode() (*NodeConfig, error) {
	connectionManager.txMu.RLock()
	defer connectionManager.txMu.RUnlock()
	if connectionManager.txIdx < 0 || connectionManager.txIdx >= len(connectionManager.txNodes) {
		return nil, fmt.Errorf("invalid tx index, not returning node")
	}
	return &connectionManager.txNodes[connectionManager.txIdx], nil
}

// internal function, switches to a node assuming a lock has been acquired
func (connectionManager *ConnectionManager) switchToNodeLocked(mode, index int, nodes []NodeConfig) (*NodeConfig, error) {
	if len(nodes) == 0 || index < 0 || index >= len(nodes) {
		return nil, fmt.Errorf("invalid node index, not switching")
	}
	if len(nodes) == 1 {
		return &nodes[0], nil
	}
	var oldIndex int
	if mode == GRPC_MODE {
		oldIndex = connectionManager.queryIdx
		connectionManager.queryIdx = index
	} else if mode == RPC_MODE {
		oldIndex = connectionManager.txIdx
		connectionManager.txIdx = index
	} else {
		log.Error().Int("mode", mode).Msg("Invalid mode, not switching")
		return nil, fmt.Errorf("invalid mode, not switching")
	}

	log.Debug().
		Str("from", nodes[oldIndex].ServerAddress).
		Str("to", nodes[index].ServerAddress).
		Msg("Switch to next node")

	return &nodes[index], nil
}

// SwitchToNextNode switches to the next node in the list.
// Node change is persistent, so it will be used again in the next call
// Returns current node if error
func (connectionManager *ConnectionManager) SwitchToNextQueryNode() (*NodeConfig, error) {
	connectionManager.queryMu.Lock()
	defer connectionManager.queryMu.Unlock()
	// Get next node index, wrap around if necessary
	nextNode := (connectionManager.queryIdx + 1) % len(connectionManager.queryNodes)
	node, err := connectionManager.switchToNodeLocked(GRPC_MODE, nextNode, connectionManager.queryNodes)
	if err != nil {
		node, err = connectionManager.GetCurrentQueryNode()
		if err != nil {
			return nil, fmt.Errorf("failed to get current query node: %w", err)
		}
	}
	return node, nil
}

// Switches to the next tx node, acquiring a lock, returning current node if error
func (connectionManager *ConnectionManager) SwitchToNextTxNode() (*NodeConfig, error) {
	connectionManager.txMu.Lock()
	defer connectionManager.txMu.Unlock()
	// Get next node index, wrap around if necessary
	nextNode := (connectionManager.txIdx + 1) % len(connectionManager.txNodes)
	node, err := connectionManager.switchToNodeLocked(RPC_MODE, nextNode, connectionManager.txNodes)
	if err != nil {
		node, err = connectionManager.GetCurrentTxNode()
		if err != nil {
			return nil, fmt.Errorf("failed to get current tx node: %w", err)
		}
	}
	return node, nil
}

// Switches to a specific node, acquiring a lock, returning current node if error
func (connectionManager *ConnectionManager) SwitchToQueryNode(index int) (*NodeConfig, error) {
	connectionManager.queryMu.Lock()
	defer connectionManager.queryMu.Unlock()
	node, err := connectionManager.switchToNodeLocked(GRPC_MODE, index, connectionManager.queryNodes)
	if err != nil {
		node, err = connectionManager.GetCurrentQueryNode()
		if err != nil {
			return nil, fmt.Errorf("failed to get current query node: %w", err)
		}
	}
	return node, nil
}

// Switches to a specific node, acquiring a lock, returning current node if error
func (connectionManager *ConnectionManager) SwitchToTxNode(index int) (*NodeConfig, error) {
	connectionManager.txMu.Lock()
	defer connectionManager.txMu.Unlock()
	node, err := connectionManager.switchToNodeLocked(RPC_MODE, index, connectionManager.txNodes)
	if err != nil {
		node, err = connectionManager.GetCurrentTxNode()
		if err != nil {
			return nil, fmt.Errorf("failed to get current tx node: %w", err)
		}
	}
	return node, nil
}

func (connectionManager *ConnectionManager) Close() error {
	log.Info().Msg("Closing ConnectionManager")
	var errors []error

	// Helper function to close a node's clients
	closeNodeClients := func(node NodeConfig) {
		if node.Chain.GRPCClient != nil {
			if err := node.Chain.GRPCClient.Close(); err != nil {
				errors = append(errors, fmt.Errorf("failed to close GRPC client for %s: %w", node.ServerAddress, err))
			}
		}
		if node.Chain.RPCClient != nil && node.Chain.RPCClient.Client != nil {
			if err := node.Chain.RPCClient.Client.Stop(); err != nil {
				errors = append(errors, fmt.Errorf("failed to stop RPC client for %s: %w", node.ServerAddress, err))
			}
		}
	}

	// Close all query nodes
	for _, node := range connectionManager.queryNodes {
		closeNodeClients(node)
	}

	// Close all tx nodes
	for _, node := range connectionManager.txNodes {
		closeNodeClients(node)
	}

	// If there were any errors, combine them into a single error message
	if len(errors) > 0 {
		var errMsg string
		for i, err := range errors {
			if i == 0 {
				errMsg = err.Error()
			} else {
				errMsg += "; " + err.Error()
			}
		}
		return fmt.Errorf("errors while closing connections: %s", errMsg)
	}

	return nil
}

func (connectionManager *ConnectionManager) SendDataWithNodeRetry(
	ctx context.Context,
	msg sdk.Msg,
	timeoutHeight uint64,
	operationName string,
) (*coretypes.ResultBroadcastTx, error) {
	return RunWithNodeRetry(ctx, connectionManager, func(node *NodeConfig) (*coretypes.ResultBroadcastTx, error) {
		return connectionManager.SendDataWithRetry(ctx, msg, operationName, timeoutHeight)
	}, operationName, RPC_MODE)
}

// RunWithNodeRetry executes an operation that returns (T, error) on nodes until success or all nodes are exhausted
func RunWithNodeRetry[T any](
	ctx context.Context,
	connectionManager ConnectionManagerInterface,
	operation func(*NodeConfig) (T, error),
	operationName string,
	mode int,
) (T, error) {
	var zeroValue T
	var err error
	var nodes []NodeConfig
	triedNodes := make(map[int]bool)
	if mode == GRPC_MODE {
		nodes, err = connectionManager.GetQueryNodes()
		if err != nil {
			return zeroValue, errorsmod.Wrapf(err, "error getting nodes")
		}
	} else if mode == RPC_MODE {
		nodes, err = connectionManager.GetTxNodes()
		if err != nil {
			return zeroValue, errorsmod.Wrapf(err, "error getting nodes")
		}
	} else {
		return zeroValue, errorsmod.Wrapf(errors.New("invalid server mode"), "invalid mode: %d, can be GRPC_MODE: %d or RPC_MODE: %d", mode, GRPC_MODE, RPC_MODE)
	}

	totalNodes := len(nodes)
	// Force change of initial node
	if mode == GRPC_MODE {
		_, err = connectionManager.SwitchToNextQueryNode()
		if err != nil {
			return zeroValue, fmt.Errorf("failed to switch to next query node: %w", err)
		}
	} else if mode == RPC_MODE {
		_, err = connectionManager.SwitchToNextTxNode()
		if err != nil {
			return zeroValue, fmt.Errorf("failed to switch to next tx node: %w", err)
		}
	}

	for attempts := 0; attempts < totalNodes; attempts++ {
		var currentNode *NodeConfig
		var currentIdx int
		if mode == GRPC_MODE {
			currentNode, err = connectionManager.GetCurrentQueryNode()
			if err != nil {
				return zeroValue, fmt.Errorf("failed to get current query node: %w", err)
			}
			currentIdx = connectionManager.GetCurrentQueryIndex() // We can use the attempt number as the index
		} else if mode == RPC_MODE {
			currentNode, err = connectionManager.GetCurrentTxNode()
			if err != nil {
				return zeroValue, fmt.Errorf("failed to get current tx node: %w", err)
			}
			currentIdx = connectionManager.GetCurrentTxIndex() // We can use the attempt number as the index
		} else {
			return zeroValue, errorsmod.Wrapf(errors.New("invalid server mode"), "invalid mode: %d, can be GRPC_MODE: %d or RPC_MODE: %d", mode, GRPC_MODE, RPC_MODE)
		}

		// Skip if we've already tried this node
		if triedNodes[currentIdx] {
			if mode == RPC_MODE {
				_, err = connectionManager.SwitchToNextQueryNode()
				if err != nil {
					return zeroValue, fmt.Errorf("failed to switch to next query node: %w", err)
				}
			} else {
				_, err = connectionManager.SwitchToNextTxNode()
				if err != nil {
					return zeroValue, fmt.Errorf("failed to switch to next tx node: %w", err)
				}
			}
			continue
		}

		// Mark this node as tried
		triedNodes[currentIdx] = true

		// Attempt operation on current node - if no error, return result
		result, err := operation(currentNode)
		if err == nil {
			return result, nil
		}

		// If it's a node switching error, switch to next node and continue
		if IsErrorSwitchingNode(err) {
			log.Warn().
				Err(err).
				Str("rpc", currentNode.ServerAddress).
				Int("idx", currentIdx). // Changed from node address to index
				Str("operation", operationName).
				Msg("Error - Switching to next node")
			if mode == GRPC_MODE {
				_, err = connectionManager.SwitchToNextQueryNode()
				if err != nil {
					return zeroValue, fmt.Errorf("failed to switch to next query node: %w", err)
				}
			} else {
				_, err = connectionManager.SwitchToNextTxNode()
				if err != nil {
					return zeroValue, fmt.Errorf("failed to switch to next tx node: %w", err)
				}
			}
			continue
		}

		// For any other error, return it immediately without switching to next node
		return zeroValue, errorsmod.Wrapf(err, "error during %s", operationName)
	}

	wallet, err := connectionManager.GetWallet()
	if err != nil {
		return zeroValue, fmt.Errorf("failed to get wallet: %w", err)
	}
	metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, wallet.Address, "", strconv.Itoa(ErrCodeAllNodesExhausted))
	return zeroValue, errorsmod.Wrapf(ErrAllNodesExhausted,
		"tried %d nodes during %s", totalNodes, operationName)
}

func validateNodeURIs(nodes []string) error {
	for _, node := range nodes {
		// Validate URI
		if node != "" {
			_, err := url.ParseRequestURI(node)
			if err != nil {
				return fmt.Errorf("invalid RPC URL %s: %w", node, err)
			}
		}
	}
	return nil
}

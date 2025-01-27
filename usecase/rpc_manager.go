package usecase

import (
	"allora_offchain_node/lib"
	"context"
	"fmt"
	"net/url"
	"sync"

	errorsmod "cosmossdk.io/errors"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog/log"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type RPCManagerInterface interface {
	GetCurrentNode() *lib.NodeConfig
	GetCurrentIndex() int
	SwitchToNextNode() *lib.NodeConfig
	SwitchToNode(index int) *lib.NodeConfig
	SendDataWithNodeRetry(ctx context.Context, msg sdk.Msg, timeoutHeight uint64, operationName string) (*cosmosclient.Response, error)
	GetNodes() ([]lib.NodeConfig, error)
}

type RPCManager struct {
	nodes      []lib.NodeConfig
	currentIdx int
	mu         sync.RWMutex
}

func NewRPCManager(userConfig lib.UserConfig) (*RPCManager, error) {
	if len(userConfig.Wallet.NodeRPCs) == 0 {
		return nil, fmt.Errorf("no RPC nodes provided")
	}

	// validate all urls are correct
	err := validateNodes(userConfig.Wallet.NodeRPCs)
	if err != nil {
		return nil, err
	}

	// Load here the nodeconfigs
	var nodes []lib.NodeConfig
	for _, rpc := range userConfig.Wallet.NodeRPCs {
		log.Info().Str("rpc", rpc).Msg("Initializing rpc")
		nodeConfig, err := userConfig.GenerateNodeConfig(rpc)
		if err != nil {
			log.Error().Err(err).Str("rpc", rpc).Msg("Error generating node config, skipping RPC node")
			continue
		}
		nodes = append(nodes, *nodeConfig)
	}

	return &RPCManager{ // nolint: exhaustruct
		nodes:      nodes,
		currentIdx: 0,
		// no need to init mu
	}, nil
}

func (r *RPCManager) GetNodes() ([]lib.NodeConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nodes, nil
}

func (r *RPCManager) GetCurrentIndex() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentIdx
}

func (r *RPCManager) GetCurrentNode() *lib.NodeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return &r.nodes[r.currentIdx]
}

// internal function, switches to a node assuming a lock has been acquired
func (r *RPCManager) switchToNodeLocked(index int) *lib.NodeConfig {
	oldIndex := r.currentIdx
	r.currentIdx = index

	log.Debug().
		Str("from", r.nodes[oldIndex].RPC).
		Str("to", r.nodes[index].RPC).
		Msg("Switch to next RPC node")

	return &r.nodes[index]
}

// SwitchToNextNode switches to the next node in the list.
// Node change is persistent, so it will be used again in the next call
func (r *RPCManager) SwitchToNextNode() *lib.NodeConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Get next node index, wrap around if necessary
	nextNode := (r.currentIdx + 1) % len(r.nodes)
	return r.switchToNodeLocked(nextNode)
}

// Switches to a specific node, acquiring a lock
func (r *RPCManager) SwitchToNode(index int) *lib.NodeConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.switchToNodeLocked(index)
}

func (r *RPCManager) SendDataWithNodeRetry(
	ctx context.Context,
	msg sdk.Msg,
	timeoutHeight uint64,
	operationName string,
) (*cosmosclient.Response, error) {
	return RunWithNodeRetry(ctx, r, func(node *lib.NodeConfig) (*cosmosclient.Response, error) {
		return node.SendDataWithRetry(ctx, msg, operationName, timeoutHeight)
	}, operationName)
}

// RunWithNodeRetry executes an operation that returns (T, error) on nodes until success or all nodes are exhausted
func RunWithNodeRetry[T any](
	ctx context.Context,
	r RPCManagerInterface,
	operation func(*lib.NodeConfig) (T, error),
	operationName string,
) (T, error) {
	var zeroValue T

	triedNodes := make(map[int]bool)
	nodes, err := r.GetNodes()
	if err != nil {
		return zeroValue, errorsmod.Wrapf(err, "error getting nodes")
	}
	totalNodes := len(nodes)
	// Force change of initial node
	r.SwitchToNextNode()

	for attempts := 0; attempts < totalNodes; attempts++ {
		currentNode := r.GetCurrentNode()
		currentIdx := r.GetCurrentIndex() // We can use the attempt number as the index

		// Skip if we've already tried this node
		if triedNodes[currentIdx] {
			r.SwitchToNextNode()
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
		if lib.IsErrorSwitchingNode(err) {
			log.Warn().
				Err(err).
				Str("rpc", currentNode.RPC).
				Int("idx", currentIdx). // Changed from node address to index
				Str("operation", operationName).
				Msg("Error - Switching to next node")
			r.SwitchToNextNode()
			continue
		}

		// For any other error, return it immediately without switching to next node
		return zeroValue, errorsmod.Wrapf(err, "error during %s", operationName)
	}

	return zeroValue, errorsmod.Wrapf(ErrAllNodesExhausted,
		"tried %d nodes during %s", totalNodes, operationName)
}

func validateNodes(nodes []string) error {
	for _, node := range nodes {
		// Validate URL
		_, err := url.ParseRequestURI(node)
		if err != nil {
			return fmt.Errorf("invalid RPC URL %s: %w", node, err)
		}
	}
	return nil
}

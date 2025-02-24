
# `lib` Package Documentation

## Overview

The `lib` package provides core functionality for the Allora off-chain node, handling configurations, RPC/GRPC connections, wallet management, and metrics collection. It serves as the foundation layer for the application's infrastructure.

## Core Components

### Configuration

- `WalletConfig`: Manages wallet-related configurations including:
  - Address management (key name, mnemonic)
  - Gas settings
  - Chain configuration
  - RPC endpoints
- `WorkerConfig`: Handles worker-specific settings, including:
  - Inference endpoints
  - Forecast configurations
  - Topic management
- `ReputerConfig`: Handles reputer-specific settings, including:
  - Ground Truth endpoint
  - IsNegative loss function endpoint
  - Topic management
- `ChainConfig`: Handles chain-specific settings, including:
  - Chain ID
  - Chain endpoints
  - Chain gas prices


### Connection Management

The lib package performs queries through GRPC and transactions through RPC. 

- `ConnectionManager`: Thread-safe manager for multiple RPC/GRPC connections
  - Handles both query and transaction nodes separately
  - Provides automatic failover and switching between nodes
  - Manages connection lifecycle
  - A ConnectionManager maintains a Wallet instance and a WalletConfig.
- `NodeConfig`: Individual node configuration and connection management. The same NodeConfig is used for both query and transaction nodes for simplicity, although they use different clients and their interfaces are different.

- GRPC/RPC operations include retry mechanisms, both intra-node and inter-node(switching between nodes). Different types of errors are handled differently to optimize retry usage via a comprehensive error handling mechanism via ABCI and HTTP error codes capturing and falling back to string matching.

### Wallet Management

- Thread-safe wallet implementation.
- Once initialized, the wallet is expected to only change its sequence number, which is done in a thread-safe manner.

### Metrics

- Singleton metrics collector for Prometheus integration
- Thread-safe counter management
- Custom metric registration

### Subpackages

Subpackages are used to encapsulate functionality that is not directly Allora. Eg. rpcclient, grpcclient, transaction boilerplate code, etc.

## Architecture Decisions

1. Singleton pattern for metrics to ensure single source of truth
2. Separation of query and transaction nodes for better reliability
3. Thread-safe implementations for concurrent operations
4. Clear separation between configuration and runtime components


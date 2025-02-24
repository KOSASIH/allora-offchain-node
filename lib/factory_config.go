package lib

import (
	grpcclient "allora_offchain_node/lib/grpcclient"
	rpcclient "allora_offchain_node/lib/rpcclient"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	cmtservice "github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	feemarkettypes "github.com/skip-mev/feemarket/x/feemarket/types"

	errorsmod "cosmossdk.io/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	cometrpc "github.com/cometbft/cometbft/rpc/client/http"
	jsonrpc "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
)

// Used
// var cdc = codec.NewProtoCodec(codectypes.NewInterfaceRegistry())

func getAlloraRPCClient(config *UserConfig, rpc string) (alloraRpcClient *rpcclient.AlloraRPCClient, err error) {
	httpClient, err := jsonrpc.DefaultHTTPClient(rpc)
	if err != nil {
		return nil, fmt.Errorf("error creating default http client")
	}

	httpClient.Timeout = time.Duration(config.Wallet.TimeoutHTTPConnection) * time.Second
	if transport, ok := httpClient.Transport.(*http.Transport); ok {
		transport.DisableKeepAlives = false
		transport.DisableCompression = false
		transport.ForceAttemptHTTP2 = true
		transport.MaxIdleConns = 100
		transport.IdleConnTimeout = 90 * time.Second
		transport.TLSHandshakeTimeout = 10 * time.Second
		transport.ExpectContinueTimeout = 1 * time.Second
	} else {
		return nil, fmt.Errorf("unexpected transport type: %T", httpClient.Transport)
	}

	cmtCli, err := cometrpc.NewWithClient(rpc, "/websocket", httpClient)
	if err != nil {
		return nil, fmt.Errorf("error creating comet rpc client")
	}

	return &rpcclient.AlloraRPCClient{Client: cmtCli}, nil
}

func (c *UserConfig) GenerateNodeConfig(ctx context.Context, wallet *Wallet, mode int, endpoint string) (nodeConfig *NodeConfig, err error) {
	log.Info().Str("endpoint", endpoint).Str("address", wallet.Address).Msg("Allora client created successfully")

	Node := NodeConfig{ // nolint: exhaustruct
		ServerAddress: endpoint,
		Chain:         ChainConfig{}, // nolint: exhaustruct
	}

	// Get RPC allora client
	var rpcClient *rpcclient.AlloraRPCClient
	if mode == RPC_MODE {
		rpcClient, err = getAlloraRPCClient(c, endpoint)
		if err != nil {
			return nil, err
		}
		Node.Chain.RPCClient = rpcClient
		Node.ServerAddress = endpoint
		log.Info().Msgf("RPC Node initialized successfully %s", endpoint)
	}

	// Get GRPC allora client
	if mode == GRPC_MODE {
		grpcConn, err := grpcclient.InitializeGRPCClient(ctx, endpoint)
		if err != nil {
			return nil, errorsmod.Wrap(err, "failed to initialize gRPC client")
		}
		Node.Chain.GRPCClient = grpcConn
		// Create query client
		Node.Chain.EmissionsQueryClient = emissionstypes.NewQueryServiceClient(grpcConn)

		Node.Chain.BankQueryClient = banktypes.NewQueryClient(grpcConn)
		Node.Chain.AuthQueryClient = authtypes.NewQueryClient(grpcConn)
		Node.Chain.FeeMarketQueryClient = feemarkettypes.NewQueryClient(grpcConn)
		Node.Chain.CometQueryClient = cmtservice.NewServiceClient(grpcConn)
		Node.Chain.TxServiceClient = txtypes.NewServiceClient(grpcConn)
		log.Info().Msgf("GRPC Node initialized successfully %s", endpoint)
	}
	return &Node, nil
}

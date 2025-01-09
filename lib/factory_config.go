package lib

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	errorsmod "cosmossdk.io/errors"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	feemarkettypes "github.com/skip-mev/feemarket/x/feemarket/types"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	jsonrpc "github.com/cometbft/cometbft/rpc/jsonrpc/client"
)

func getAlloraClient(config *UserConfig) (*cosmosclient.Client, error) {
	// create a allora client instance
	ctx := context.Background()
	userHomeDir, _ := os.UserHomeDir()
	alloraClientHome := filepath.Join(userHomeDir, ".allorad")
	if config.Wallet.AlloraHomeDir != "" {
		alloraClientHome = config.Wallet.AlloraHomeDir
	}

	// Check that the given home folder exists
	if _, err := os.Stat(alloraClientHome); errors.Is(err, os.ErrNotExist) {
		log.Info().Msg("Home directory does not exist, creating...")
		err = os.MkdirAll(alloraClientHome, 0755)
		if err != nil {
			config.Wallet.SubmitTx = false
			return nil, errorsmod.Wrap(err, "cannot create allora client home directory")
		}
		log.Info().Str("home", alloraClientHome).Msg("Allora client home directory created")
	}

	httpClient, err := jsonrpc.DefaultHTTPClient(config.Wallet.NodeRpc)
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

	rpcClient, err := rpchttp.NewWithClient(config.Wallet.NodeRpc, "/websocket", httpClient)
	if err != nil {
		return nil, fmt.Errorf("error creating rpc client")
	}

	client, err := cosmosclient.New(ctx,
		cosmosclient.WithNodeAddress(config.Wallet.NodeRpc),
		cosmosclient.WithAddressPrefix(ADDRESS_PREFIX),
		cosmosclient.WithHome(alloraClientHome),
		cosmosclient.WithGas(config.Wallet.Gas),
		cosmosclient.WithGasAdjustment(config.Wallet.GasAdjustment),
		cosmosclient.WithAccountRetriever(authtypes.AccountRetriever{}),
		cosmosclient.WithRPCClient(rpcClient),
	)
	if err != nil {
		config.Wallet.SubmitTx = false
		return nil, err
	}
	return &client, nil
}

func (c *UserConfig) GenerateNodeConfig() (*NodeConfig, error) {
	client, err := getAlloraClient(c)
	if err != nil {
		c.Wallet.SubmitTx = false
		return nil, err
	}
	var account *cosmosaccount.Account
	// if we're giving a keyring ring name, with no mnemonic restore
	if c.Wallet.AddressRestoreMnemonic == "" && c.Wallet.AddressKeyName != "" {
		// get account from the keyring
		acc, err := client.Account(c.Wallet.AddressKeyName)
		if err != nil {
			c.Wallet.SubmitTx = false
			log.Error().Err(err).Msg("could not retrieve account from keyring")
		} else {
			account = &acc
		}
	} else if c.Wallet.AddressRestoreMnemonic != "" && c.Wallet.AddressKeyName != "" {
		// restore from mnemonic
		acc, err := client.AccountRegistry.Import(c.Wallet.AddressKeyName, c.Wallet.AddressRestoreMnemonic, "")
		if err != nil {
			if err.Error() == "account already exists" {
				acc, err = client.Account(c.Wallet.AddressKeyName)
			}

			if err != nil {
				c.Wallet.SubmitTx = false
				log.Err(err).Msg("could not restore account from mnemonic")
			} else {
				account = &acc
			}
		} else {
			account = &acc
		}
	} else {
		return nil, errors.New("no allora account was loaded")
	}

	if account == nil {
		return nil, errors.New("no allora account was loaded")
	}

	address, err := account.Address(ADDRESS_PREFIX)
	if err != nil {
		c.Wallet.SubmitTx = false
		log.Err(err).Msg("could not retrieve allora blockchain address, transactions will not be submitted to chain")
	} else {
		log.Info().Str("address", address).Msg("allora blockchain address loaded")
	}

	// Create query client
	queryClient := emissionstypes.NewQueryServiceClient(client.Context())

	// Create bank client
	bankClient := banktypes.NewQueryClient(client.Context())

	// Where other clients are initialized, add:
	feeMarketQueryClient := feemarkettypes.NewQueryClient(client.Context())

	// Check chainId is set
	if client.Context().ChainID == "" {
		return nil, errors.New("ChainId is empty")
	}

	c.Wallet.Address = address // Overwrite the address with the one from the keystore

	log.Info().Msg("Allora client created successfully")
	log.Info().Msg("Wallet address: " + address)

	alloraChain := ChainConfig{
		Address:              address,
		AddressPrefix:        ADDRESS_PREFIX,
		DefaultBondDenom:     DEFAULT_BOND_DENOM,
		Account:              *account,
		Client:               client,
		EmissionsQueryClient: queryClient,
		BankQueryClient:      bankClient,
		FeeMarketQueryClient: feeMarketQueryClient,
	}

	Node := NodeConfig{
		Chain:   alloraChain,
		Wallet:  c.Wallet,
		Worker:  c.Worker,
		Reputer: c.Reputer,
	}

	return &Node, nil
}

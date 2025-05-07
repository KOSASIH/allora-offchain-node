package lib

import (
	"allora_offchain_node/lib/rpcclient"
	"errors"
	"fmt"

	emissions "github.com/allora-network/allora-chain/x/emissions/types"
	cmtservice "github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	auth "github.com/cosmos/cosmos-sdk/x/auth/types"
	bank "github.com/cosmos/cosmos-sdk/x/bank/types"
	feemarkettypes "github.com/skip-mev/feemarket/x/feemarket/types"
	"google.golang.org/grpc"
)

const AutoGasPrices = "auto"

// Minimum values
const (
	WindowCorrectionFactorSuggestedMin = 0.5
	BlockDurationEstimatedMin          = 1.0
	GasPriceUpdateIntervalMin          = 5
	RetryDelayMin                      = 1
	AccountSequenceRetryDelayMin       = 1
	RegistrationWaitingBlocksMin       = 1
)

// Default values
const (
	DefaultTimeoutRPCSecondsQuery        int64   = 60
	DefaultTimeoutRPCSecondsTx           int64   = 300
	DefaultTimeoutRPCSecondsRegistration int64   = 300
	DefaultTimeoutHTTPConnection         int64   = 10
	DefaultGasPriceUpdateInterval        int64   = 60
	DefaultLaunchRoutineDelay            int64   = 5
	DefaultRetryDelay                    int64   = 3
	DefaultAccountSequenceRetryDelay     int64   = 5
	DefaultBaseGas                       uint64  = 200000
	DefaultGasPerByte                    uint64  = 1
	DefaultKeyringBackend                string  = "test"
	DefaultGasAdjustment                 float64 = 1.2
	DefaultSimulateGasFromStart          bool    = false
	DefaultGrpcInsecure                  bool    = false
	DefaultRegistrationWaitingBlocks     int64   = 5
)

// Properties manually provided by the user as part of UserConfig
type WalletConfig struct {
	// Provided by the user
	AddressKeyName                string                  // load a address by key from the keystore
	AddressRestoreMnemonic        string                  // load a address by mnemonic from the keystore
	AlloraHomeDir                 string                  // home directory for the allora keystore
	ChainId                       string                  // chain id
	KeyringBackend                string                  // keyring backend to use ("test", "os", "file", ...)
	KeyringPassphrase             string                  // passphrase for the keyring (if needed)
	GasPrices                     string                  // gas prices to use for the allora client - "auto" for auto-calculated fees
	GasPriceUpdateInterval        int64                   // number of seconds to wait between updates to the gas price
	GasAdjustment                 float64                 // adjustment factor for the gas used
	SimulateGasFromStart          bool                    // true: simulate gas on first try, false: simulate gas on retry only
	MaxFees                       FlexibleCosmosIntAmount // max fees to pay for a single transaction (as string or number)
	BaseGas                       uint64                  // base gas to use for the allora client
	GasPerByte                    uint64                  // gas per byte to use for the allora client
	NodeRPCs                      []string                // rpc nodes for allora chain
	NodeGRPCs                     []string                // grpc nodes for allora chain
	MaxRetries                    int64                   // retry to get data from chain up to this many times per query or tx
	RetryDelay                    int64                   // number of seconds to wait between retries (general case)
	AccountSequenceRetryDelay     int64                   // number of seconds to wait between retries in case of account sequence error
	LaunchRoutineDelay            int64                   // number of seconds to wait between starting a routine and the next one to avoid 429 errors
	SubmitTx                      bool                    // useful for dev/testing. set to false to run in dry-run processes without committing to the chain
	BlockDurationEstimated        float64                 // estimated average block duration in seconds
	RegistrationWaitingBlocks     int64                   // number of blocks to wait for a registration to be included in a block
	WindowCorrectionFactor        float64                 // correction factor for the time estimation, suggested range 0.7-0.9.
	TimeoutRPCSecondsQuery        int64                   // timeout for rpc queries in seconds, including retries
	TimeoutRPCSecondsTx           int64                   // timeout for rpc data send in seconds, including retries
	TimeoutRPCSecondsRegistration int64                   // timeout for rpc registration in seconds, including retries
	TimeoutHTTPConnection         int64                   // timeout for http connection in seconds
	GrpcInsecure                  bool                    // use insecure grpc connection
}

// Communication with the chain
type ChainConfig struct {
	RPCClient            *rpcclient.AlloraRPCClient // A custom wrapper around the cometrpc.HTTP client
	GRPCClient           *grpc.ClientConn           // Basic type to be used to init module-based clients
	EmissionsQueryClient emissions.QueryServiceClient
	BankQueryClient      bank.QueryClient
	AuthQueryClient      auth.QueryClient
	FeeMarketQueryClient feemarkettypes.QueryClient
	CometQueryClient     cmtservice.ServiceClient
	TxServiceClient      txtypes.ServiceClient
}

type TopicActor interface {
	GetTopicId() emissions.TopicId
}

type WorkerConfig struct {
	TopicId                 emissions.TopicId
	InferenceEntrypointName string
	InferenceEntrypoint     AlloraAdapter
	ForecastEntrypointName  string
	ForecastEntrypoint      AlloraAdapter     // seconds to wait between attempts to get next worker nonce
	Parameters              map[string]string // Map for variable configuration values
}

// Implement TopicActor interface for WorkerConfig
func (workerConfig WorkerConfig) GetTopicId() emissions.TopicId {
	return workerConfig.TopicId
}

type ReputerConfig struct {
	TopicId                    emissions.TopicId
	GroundTruthEntrypointName  string
	GroundTruthEntrypoint      AlloraAdapter
	LossFunctionEntrypointName string
	LossFunctionEntrypoint     AlloraAdapter
	// Minimum stake to repute. will try to add stake from wallet if current stake is less than this.
	// Will not repute if current stake is less than this, after trying to add any necessary stake.
	// This is idempotent in that it will not add more stake than specified here.
	// Set to 0 to effectively disable this feature and use whatever stake has already been added.
	MinStake               FlexibleCosmosIntAmount
	GroundTruthParameters  map[string]string      // Map for variable configuration values
	LossFunctionParameters LossFunctionParameters // Map for variable configuration values
}

// Implement TopicActor interface for ReputerConfig
func (reputerConfig ReputerConfig) GetTopicId() emissions.TopicId {
	return reputerConfig.TopicId
}

type LossFunctionParameters struct {
	LossFunctionService string
	LossMethodOptions   map[string]string
	IsNeverNegative     *bool // Cached result of whether the loss function is never negative
}

type UserConfig struct {
	Wallet  WalletConfig
	Worker  []WorkerConfig
	Reputer []ReputerConfig
}

// NodeConfig is the configuration for a node
type NodeConfig struct {
	ServerAddress     string             // Server endpoint address URI
	Chain             ChainConfig        // Configuration for the chain
	ConnectionManager *ConnectionManager // Link to the ConnectionManager that created this node
}

type WorkerResponse struct {
	WorkerConfig
	InfererValue     string      `json:"infererValue,omitempty"`
	ForecasterValues []NodeValue `json:"forecasterValue,omitempty"`
}

type SignedWorkerResponse struct {
	*emissions.WorkerDataBundle
	BlockHeight int64 `json:"blockHeight,omitempty"`
	TopicId     int64 `json:"topicId,omitempty"`
}

type ValueBundle struct {
	CombinedValue          string      `json:"combinedValue,omitempty"`
	NaiveValue             string      `json:"naiveValue,omitempty"`
	InfererValues          []NodeValue `json:"infererValues,omitempty"`
	ForecasterValues       []NodeValue `json:"forecasterValues,omitempty"`
	OneOutInfererValues    []NodeValue `json:"oneOutInfererValues,omitempty"`
	OneOutForecasterValues []NodeValue `json:"oneOutForecasterValues,omitempty"`
	OneInForecasterValues  []NodeValue `json:"oneInForecasterValues,omitempty"`
}

// Check and set defaults for the user config if any values are not set
func (c *UserConfig) CheckAndSetDefaults() {
	if c.Wallet.TimeoutRPCSecondsQuery == 0 {
		c.Wallet.TimeoutRPCSecondsQuery = DefaultTimeoutRPCSecondsQuery
	}
	if c.Wallet.TimeoutRPCSecondsTx == 0 {
		c.Wallet.TimeoutRPCSecondsTx = DefaultTimeoutRPCSecondsTx
	}
	if c.Wallet.TimeoutRPCSecondsRegistration == 0 {
		c.Wallet.TimeoutRPCSecondsRegistration = DefaultTimeoutRPCSecondsRegistration
	}
	if c.Wallet.GasPriceUpdateInterval == 0 {
		c.Wallet.GasPriceUpdateInterval = DefaultGasPriceUpdateInterval
	}
	if c.Wallet.LaunchRoutineDelay == 0 {
		c.Wallet.LaunchRoutineDelay = DefaultLaunchRoutineDelay
	}
	if c.Wallet.RetryDelay == 0 {
		c.Wallet.RetryDelay = DefaultRetryDelay
	}
	if c.Wallet.TimeoutHTTPConnection == 0 {
		c.Wallet.TimeoutHTTPConnection = DefaultTimeoutHTTPConnection
	}
	if c.Wallet.BaseGas == 0 {
		c.Wallet.BaseGas = DefaultBaseGas
	}
	if c.Wallet.GasPerByte == 0 {
		c.Wallet.GasPerByte = DefaultGasPerByte
	}
	if c.Wallet.KeyringBackend == "" {
		c.Wallet.KeyringBackend = DefaultKeyringBackend
	}
	if c.Wallet.GasAdjustment == 0 {
		c.Wallet.GasAdjustment = DefaultGasAdjustment
	}
	if c.Wallet.RegistrationWaitingBlocks == 0 {
		c.Wallet.RegistrationWaitingBlocks = DefaultRegistrationWaitingBlocks
	}
}

// Check that each assigned entrypoint in the user config actually can be used
// for the intended purpose, else throw error
func (c *UserConfig) ValidateConfigAdapters() error {
	// Validate wallet config
	err := c.ValidateWalletConfig()
	if err != nil {
		return err
	}
	// Validate worker configs
	for _, workerConfig := range c.Worker {
		err := workerConfig.ValidateWorkerConfig()
		if err != nil {
			return err
		}
	}
	// Validate reputer configs
	for _, reputerConfig := range c.Reputer {
		err := reputerConfig.ValidateReputerConfig()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *UserConfig) ValidateWalletConfig() error {
	if c.Wallet.WindowCorrectionFactor < WindowCorrectionFactorSuggestedMin {
		return fmt.Errorf("window correction factor lower than suggested minimum: %f < %f", c.Wallet.WindowCorrectionFactor, WindowCorrectionFactorSuggestedMin)
	}
	if c.Wallet.BlockDurationEstimated < BlockDurationEstimatedMin {
		return fmt.Errorf("block duration estimated lower than the minimum: %f < %f", c.Wallet.BlockDurationEstimated, BlockDurationEstimatedMin)
	}
	if c.Wallet.RetryDelay < RetryDelayMin {
		return fmt.Errorf("retry delay lower than the minimum: %d < %d", c.Wallet.RetryDelay, RetryDelayMin)
	}
	if c.Wallet.AccountSequenceRetryDelay < AccountSequenceRetryDelayMin {
		return fmt.Errorf("account sequence retry delay lower than the minimum: %d < %d", c.Wallet.AccountSequenceRetryDelay, AccountSequenceRetryDelayMin)
	}
	if c.Wallet.GasPrices == AutoGasPrices && c.Wallet.GasPriceUpdateInterval < GasPriceUpdateIntervalMin {
		return fmt.Errorf("gas price update interval (in 'auto' mode)lower than the minimum: %d < %d", c.Wallet.GasPriceUpdateInterval, GasPriceUpdateIntervalMin)
	}
	if c.Wallet.TimeoutHTTPConnection < 0 {
		return fmt.Errorf("invalid timeout http connection: %d", c.Wallet.TimeoutHTTPConnection)
	}
	if c.Wallet.TimeoutRPCSecondsRegistration < 0 {
		return fmt.Errorf("invalid timeout rpc seconds registration: %d", c.Wallet.TimeoutRPCSecondsRegistration)
	}
	if c.Wallet.TimeoutRPCSecondsQuery < 0 {
		return fmt.Errorf("invalid timeout rpc seconds query: %d", c.Wallet.TimeoutRPCSecondsQuery)
	}
	if c.Wallet.TimeoutRPCSecondsTx < 0 {
		return fmt.Errorf("invalid timeout rpc seconds tx: %d", c.Wallet.TimeoutRPCSecondsTx)
	}
	if c.Wallet.ChainId == "" {
		return fmt.Errorf("chain id is empty")
	}
	if c.Wallet.GasAdjustment <= 0 {
		return fmt.Errorf("gas adjustment must be greater than 0: %f", c.Wallet.GasAdjustment)
	}
	if c.Wallet.RegistrationWaitingBlocks < RegistrationWaitingBlocksMin {
		return fmt.Errorf("registration waiting blocks lower than the minimum: %d < %d", c.Wallet.RegistrationWaitingBlocks, RegistrationWaitingBlocksMin)
	}
	return nil
}

func (reputerConfig *ReputerConfig) ValidateReputerConfig() error {
	if reputerConfig.GroundTruthEntrypointName == "" ||
		reputerConfig.GroundTruthEntrypoint == nil ||
		(reputerConfig.GroundTruthEntrypoint != nil &&
			!reputerConfig.GroundTruthEntrypoint.CanSourceGroundTruthAndComputeLoss()) {
		return errors.New("invalid ground truth entrypoint")
	}
	if reputerConfig.LossFunctionEntrypointName == "" ||
		reputerConfig.LossFunctionEntrypoint == nil {
		return errors.New("invalid loss function entrypoint")
	}
	return nil
}

func (workerConfig *WorkerConfig) ValidateWorkerConfig() error {
	if workerConfig.InferenceEntrypoint != nil && !workerConfig.InferenceEntrypoint.CanInfer() {
		return errors.New("invalid inference entrypoint")
	}
	if workerConfig.ForecastEntrypoint != nil && !workerConfig.ForecastEntrypoint.CanForecast() {
		return errors.New("invalid forecast entrypoint")
	}
	return nil
}

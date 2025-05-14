package lib

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	metrics "allora_offchain_node/metrics"

	errorsmod "cosmossdk.io/errors"
	emissions "github.com/allora-network/allora-chain/x/emissions/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/rs/zerolog/log"
	feemarkettypes "github.com/skip-mev/feemarket/x/feemarket/types"
)

// Error codes for the module
const ErrorCodespace = "allora-offchain-lib"

// Error codes for the module and also emitted in metrics. Some may only be emitted in metrics.
const ErrCodeHTTP = 1
const ErrCodeNotEnoughBalance = 2
const ErrCodeNotRegistered = 3
const ErrCodeStakeBelowMin = 4
const ErrCodeFullMempool = 5
const ErrCodeReadPanic = 6
const ErrCodeConnectionRefused = 7
const ErrCodeAllNodesExhausted = 8
const ErrCodeTxSimulationError = 9
const ErrCodeCannotAddStake = 10
const ErrCodeAccountSequenceMismatch = 11
const ErrCodeInsufficientFees = 12
const ErrCodeOutOfGas = 13
const ErrCodeNoFeeCoins = 14
const ErrCodeTxTooLarge = 15
const ErrCodeTxInMempoolCache = 16
const ErrCodeInvalidChainID = 17
const ErrCodeTxTimeoutHeight = 18
const ErrCodeWorkerNonceWindowNotAvailable = 19
const ErrCodeReputerNonceWindowNotAvailable = 20
const ErrCodeContextDeadlineExceededTimeout = 21
const ErrCodeNoInferencesFoundForTopic = 22
const ErrCodeNotPermittedToSubmitPayload = 23
const ErrCodeNotPermittedToAddStake = 24
const ErrCodeReadFlatPanic = 25
const ErrCodeReadPerBytePanic = 26
const ErrCodeUnexpectedError = 100

var (
	ErrHTTP                           = errorsmod.Register(ErrorCodespace, ErrCodeHTTP, "http error")
	ErrNotEnoughBalance               = errorsmod.Register(ErrorCodespace, ErrCodeNotEnoughBalance, "not enough balance")
	ErrNotRegistered                  = errorsmod.Register(ErrorCodespace, ErrCodeNotRegistered, "not registered")
	ErrStakeBelowMin                  = errorsmod.Register(ErrorCodespace, ErrCodeStakeBelowMin, "stake below minimum")
	ErrFullMempool                    = errorsmod.Register(ErrorCodespace, ErrCodeFullMempool, "full mempool")
	ErrReadPanic                      = errorsmod.Register(ErrorCodespace, ErrCodeReadPanic, "read panic")
	ErrConnectionRefused              = errorsmod.Register(ErrorCodespace, ErrCodeConnectionRefused, "connection refused")
	ErrAllNodesExhausted              = errorsmod.Register(ErrorCodespace, ErrCodeAllNodesExhausted, "all available nodes have been tried and exhausted")
	ErrTxSimulationError              = errorsmod.Register(ErrorCodespace, ErrCodeTxSimulationError, "Tx simulation error")
	ErrCannotAddStake                 = errorsmod.Register(ErrorCodespace, ErrCodeCannotAddStake, "not permitted to add stake")
	ErrAccountSequenceMismatch        = errorsmod.Register(ErrorCodespace, ErrCodeAccountSequenceMismatch, "account sequence mismatch")
	ErrInsufficientFees               = errorsmod.Register(ErrorCodespace, ErrCodeInsufficientFees, "insufficient fees")
	ErrOutOfGas                       = errorsmod.Register(ErrorCodespace, ErrCodeOutOfGas, "out of gas")
	ErrNoFeeCoins                     = errorsmod.Register(ErrorCodespace, ErrCodeNoFeeCoins, "no fee coins (bad tx)")
	ErrUnexpectedError                = errorsmod.Register(ErrorCodespace, ErrCodeUnexpectedError, "unexpected error")
	ErrContextDeadlineExceededTimeout = errorsmod.Register(ErrorCodespace, ErrCodeContextDeadlineExceededTimeout, "context deadline exceeded timeout")
	ErrReputerNonceWindowNotAvailable = errorsmod.Register(ErrorCodespace, ErrCodeReputerNonceWindowNotAvailable, "reputer nonce window not available")
	ErrWorkerNonceWindowNotAvailable  = errorsmod.Register(ErrorCodespace, ErrCodeWorkerNonceWindowNotAvailable, "worker nonce window not available")
	ErrNoInferencesFoundForTopic      = errorsmod.Register(ErrorCodespace, ErrCodeNoInferencesFoundForTopic, "no inferences found for topic")
)

// Errors substrings that are not ABCI errors and do not have a specific error code
const ErrorMessageDataAlreadySubmitted = "already submitted"
const ErrorMessageCannotUpdateEma = "cannot update EMA"
const ErrorMessageWaitingForNextBlock = "waiting for next block" // This means tx is accepted in mempool but not yet included in a block
const ErrorMessageAccountSequenceMismatch = "account sequence mismatch"
const ErrorMessageTimeoutHeight = "timeout height"
const ErrorMessageNotPermittedToSubmitPayload = "not permitted to submit payload"
const ErrorMessageNotPermittedToAddStake = "not permitted to add stake"
const ErrorMessageReadFlatPanic = "{ReadFlat}: panic"
const ErrorMessageReadPerBytePanic = "{ReadPerByte}: panic"
const ErrorMessageConnectionRefused = "connection refused"
const ErrorMessageNoInferencesFoundForTopic = "no inferences found for topic"
const ErrorContextDeadlineExceeded = "context deadline exceeded"
const ErrorReputerNonceWindowNotAvailable = "reputer nonce window not available"
const ErrorWorkerNonceWindowNotAvailable = "worker nonce window not available"

// Error processing types
// - "continue", nil: tx was not successful, but special error type. Handled, ready for retry
// - "ok", nil: tx was successful, error handled and not re-raised
// - "error", error: tx failed, with regular error type
// - "fees": tx failed, because of insufficient fees
// - "failure": tx failed, and should not be retried anymore
// - "switch": tx failed, and should be retried with a different node
const ErrorProcessingContinue = "continue"
const ErrorProcessingOk = "ok"
const ErrorProcessingFees = "fees"
const ErrorProcessingGas = "gas"
const ErrorProcessingError = "error"
const ErrorProcessingFailure = "failure"
const ErrorProcessingSwitchingNode = "switch"
const ErrorProcessingResetSequence = "reset_sequence"

// HTTP status codes that trigger node switching
var HTTPStatusCodeCodesSwitchingNode = map[int]bool{
	403: true, // Forbidden
	429: true, // Too Many Requests
	502: true, // Bad Gateway
	503: true, // Service Unavailable
	504: true, // Gateway Timeout
	505: true, // HTTP Version Not Supported
}

const GAS_EXCESS_CORRECTION uint64 = 20000

// calculateExponentialBackoffDelay returns a duration based on retry count and base delay
func calculateExponentialBackoffDelaySeconds(baseDelay int64, retryCount int64) int64 {
	return int64(math.Pow(float64(baseDelay), float64(retryCount)))
}

// extractErrorCode attempts to extract an ABCI error code from an error message
// Returns the error code and true if successful, 0 and false otherwise
func extractErrorCode(errorMessage string) (uint32, bool) {
	re := regexp.MustCompile(`error code:?\s*'?(\d+)'?:?`)
	matches := re.FindStringSubmatch(errorMessage)
	if len(matches) != 2 {
		return 0, false
	}

	errorCode, err := strconv.ParseUint(matches[1], 10, 32)
	if err != nil || errorCode > math.MaxUint32 {
		return 0, false
	}

	// parseuint cannot be done on uint32 directly, but it is caught by the checks above
	return uint32(errorCode), true // nolint:gosec
}

// ProcessErrorTx handles the error messages.
func ProcessErrorTx(ctx context.Context, err error, infoMsg string, retryCount, retryMax int64, node *NodeConfig) (string, error) {
	if errorCode, ok := extractErrorCode(err.Error()); ok {
		return triageABCIErrorCode(ctx, errorCode, err, infoMsg, retryCount, retryMax, node)
	}

	// Check if error is HTTP status code
	if processingType, err := triageHTTPStatusError(err, node, infoMsg); err != nil {
		return processingType, err
	}

	return triageStringMatchingError(ctx, err, infoMsg, node)
}

// triageABCIErrorCode handles specific ABCI error codes and returns appropriate processing instructions
func triageABCIErrorCode(ctx context.Context, errorCode uint32, err error, infoMsg string, retryCount, retryMax int64, node *NodeConfig) (string, error) {
	connectionManager := node.ConnectionManager
	// Beware: this error must not overwrite the "err" error
	walletConfig, errorWalletConfig := connectionManager.GetWalletConfig()
	if errorWalletConfig != nil {
		return "", errorWalletConfig
	}
	switch errorCode {
	case sdkerrors.ErrMempoolIsFull.ABCICode():
		// Exhaust retries before switching to next node
		if retryCount >= retryMax {
			log.Info().
				Err(err).
				Str("msg", infoMsg).
				Msg("Mempool is full, switching to next node")
			return ErrorProcessingSwitchingNode, ErrFullMempool
		} else {
			delay := calculateExponentialBackoffDelaySeconds(walletConfig.RetryDelay, retryCount)
			if DoneOrWait(ctx, delay) {
				return ErrorProcessingError, ctx.Err()
			}
			log.Info().
				Err(err).
				Str("msg", infoMsg).
				Msg("Mempool is full, retrying with exponential backoff")
			metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeFullMempool))
			return ErrorProcessingContinue, nil
		}
	case sdkerrors.ErrWrongSequence.ABCICode(), sdkerrors.ErrInvalidSequence.ABCICode():
		log.Warn().
			Err(err).
			Str("msg", infoMsg).
			Int64("delay", walletConfig.AccountSequenceRetryDelay).
			Msg("Account sequence mismatch detected, re-fetching sequence")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeAccountSequenceMismatch))
		return parseAndSetNewWalletSequence(ctx, err, node, infoMsg)
	case sdkerrors.ErrInsufficientFee.ABCICode():
		log.Info().
			Err(err).
			Str("msg", infoMsg).
			Msg("Insufficient fees")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeInsufficientFees))
		return ErrorProcessingFees, nil
	case sdkerrors.ErrOutOfGas.ABCICode():
		log.Info().
			Err(err).
			Str("msg", infoMsg).
			Msg("Out of gas - increase your base gas")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeOutOfGas))
		return ErrorProcessingGas, nil
	case feemarkettypes.ErrNoFeeCoins.ABCICode():
		log.Info().
			Err(err).
			Str("msg", infoMsg).
			Msg("No fee coins")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeNoFeeCoins))
		return ErrorProcessingFailure, nil
	case sdkerrors.ErrTxTooLarge.ABCICode():
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeTxTooLarge))
		return ErrorProcessingError, errorsmod.Wrapf(err, "tx too large")
	case sdkerrors.ErrTxInMempoolCache.ABCICode():
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeTxInMempoolCache))
		return ErrorProcessingError, errorsmod.Wrapf(err, "tx already in mempool cache")
	case sdkerrors.ErrInvalidChainID.ABCICode():
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeInvalidChainID))
		return ErrorProcessingError, errorsmod.Wrapf(err, "invalid chain-id")
	case sdkerrors.ErrTxTimeoutHeight.ABCICode():
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeTxTimeoutHeight))
		return ErrorProcessingFailure, errorsmod.Wrapf(err, "tx timeout height")
	case emissions.ErrWorkerNonceWindowNotAvailable.ABCICode():
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeWorkerNonceWindowNotAvailable))
		log.Warn().
			Err(err).
			Str("msg", infoMsg).
			Msg("Worker window not available, retrying with exponential backoff")
		delay := calculateExponentialBackoffDelaySeconds(walletConfig.RetryDelay, retryCount)
		if DoneOrWait(ctx, delay) {
			return ErrorProcessingError, ctx.Err()
		}
		return ErrorProcessingContinue, nil
	case emissions.ErrReputerNonceWindowNotAvailable.ABCICode():
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeReputerNonceWindowNotAvailable))
		log.Warn().
			Err(err).
			Str("msg", infoMsg).
			Msg("Reputer window not available, retrying with exponential backoff")
		delay := calculateExponentialBackoffDelaySeconds(walletConfig.RetryDelay, retryCount)
		if DoneOrWait(ctx, delay) {
			return ErrorProcessingError, ctx.Err()
		}
		return ErrorProcessingContinue, nil
	default:
		log.Info().Uint32("errorCode", errorCode).Str("msg", infoMsg).Msg("ABCI error, but not special case - regular retry")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeUnexpectedError))
		return ErrorProcessingError, err
	}
}

// Triages error by string matching
func triageStringMatchingError(ctx context.Context, err error, infoMsg string, node *NodeConfig) (string, error) {
	connectionManager := node.ConnectionManager
	walletConfig, errorWalletConfig := connectionManager.GetWalletConfig()
	if errorWalletConfig != nil {
		return "", errorWalletConfig
	}

	if strings.Contains(err.Error(), ErrorMessageAccountSequenceMismatch) {
		log.Warn().
			Err(err).
			Str("rpc", node.ServerAddress).
			Str("msg", infoMsg).
			Int64("delay", walletConfig.AccountSequenceRetryDelay).
			Msg("Account sequence mismatch detected, re-fetching sequence")
		return parseAndSetNewWalletSequence(ctx, err, node, infoMsg)

	} else if strings.Contains(err.Error(), ErrorContextDeadlineExceeded) {
		log.Warn().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("Context deadline exceeded, switching to next node")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeContextDeadlineExceededTimeout))
		return ErrorProcessingSwitchingNode, err
	} else if strings.Contains(err.Error(), ErrorMessageWaitingForNextBlock) {
		log.Warn().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("Tx accepted in mempool, it will be included in the following block(s) - not retrying")
		return ErrorProcessingOk, nil
	} else if strings.Contains(err.Error(), ErrorMessageDataAlreadySubmitted) || strings.Contains(err.Error(), ErrorMessageCannotUpdateEma) {
		log.Warn().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("Already submitted data for this epoch.")
		return ErrorProcessingOk, nil
	} else if strings.Contains(err.Error(), ErrorMessageTimeoutHeight) {
		log.Warn().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("Tx failed because of timeout height")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeTxTimeoutHeight))
		return ErrorProcessingFailure, err
	} else if strings.Contains(err.Error(), ErrorMessageNotPermittedToSubmitPayload) {
		log.Warn().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("Actor is not permitted to submit payload")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeNotPermittedToSubmitPayload))
		return ErrorProcessingFailure, err
	} else if strings.Contains(err.Error(), ErrorMessageNoInferencesFoundForTopic) {
		log.Warn().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("No inferences found for topic")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeNoInferencesFoundForTopic))
		return ErrorProcessingFailure, err
	} else if strings.Contains(err.Error(), ErrorMessageNotPermittedToAddStake) {
		log.Warn().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("Actor is not permitted to add stake")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeNotPermittedToAddStake))
		return ErrorProcessingFailure, err
	} else if strings.Contains(err.Error(), ErrorMessageReadFlatPanic) || strings.Contains(err.Error(), ErrorMessageReadPerBytePanic) {
		log.Warn().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("Read panic, switching to next node")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeReadPanic))
		return ErrorProcessingSwitchingNode, ErrReadPanic
	} else if strings.Contains(err.Error(), ErrorMessageConnectionRefused) {
		log.Warn().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("Connection refused, switching to next node")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeConnectionRefused))
		return ErrorProcessingSwitchingNode, ErrConnectionRefused
	} else if strings.Contains(err.Error(), ErrorReputerNonceWindowNotAvailable) {
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeReputerNonceWindowNotAvailable))
		return ErrorProcessingContinue, ErrReputerNonceWindowNotAvailable
	} else if strings.Contains(err.Error(), ErrorWorkerNonceWindowNotAvailable) {
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeWorkerNonceWindowNotAvailable))
		return ErrorProcessingContinue, ErrWorkerNonceWindowNotAvailable
	}
	log.Info().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msg("Unknown error")
	metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeUnexpectedError))
	return ErrorProcessingError, errorsmod.Wrap(ErrUnexpectedError, err.Error())
}

// triageHTTPStatusError checks if the error contains an HTTP status code and determines if node switching is needed
func triageHTTPStatusError(err error, node *NodeConfig, infoMsg string) (string, error) {
	statusCode, statusMessage, parseErr := ParseHTTPStatus(err.Error())
	if parseErr == nil {
		log.Info().
			Int("statusCode", statusCode).
			Str("statusMessage", statusMessage).
			Str("msg", infoMsg).
			Msg("HTTP status error code detected")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, node.ConnectionManager.wallet.Address, strconv.Itoa(ErrCodeHTTP))

		// When status code is in the list of codes that trigger node switching, switch to next node without retries
		if HTTPStatusCodeCodesSwitchingNode[statusCode] {
			log.Warn().
				Str("rpc", node.ServerAddress).
				Int("statusCode", statusCode).
				Str("statusMessage", statusMessage).
				Str("msg", infoMsg).
				Msg("HTTP status error code detected, switching to next node")
			return ErrorProcessingSwitchingNode, ErrHTTP
		}
	}
	return "", nil
}

// ParseHTTPStatus extracts HTTP status code and message from an error string
func ParseHTTPStatus(input string) (int, string, error) {
	// Updated regex to be less greedy and handle the standard HTTP status format
	re := regexp.MustCompile(`(?i)Status:\s*(\d+)(?:\s+([^-]+))?`)
	matches := re.FindStringSubmatch(input)

	if len(matches) < 2 {
		return 0, "", fmt.Errorf("invalid status format")
	}

	code, err := strconv.Atoi(matches[1])
	if err != nil || code < 0 {
		return 0, "", fmt.Errorf("invalid status code")
	}

	// Clean up the status message by trimming spaces
	message := ""
	if len(matches) > 2 && matches[2] != "" {
		message = strings.TrimSpace(matches[2])
	}

	return code, message, nil
}

// Returns true if the error is a switching-node error
func IsErrorSwitchingNode(err error) bool {
	return errors.Is(err, ErrHTTP) ||
		errors.Is(err, ErrFullMempool) ||
		errors.Is(err, ErrReadPanic) ||
		errors.Is(err, ErrConnectionRefused) ||
		errors.Is(err, ErrUnexpectedError)
}

// Extract expected and current sequence numbers from account sequence mismatch error message
func parseSequenceFromAccountMismatchError(errorMessage string) (expected uint64, current uint64, err error) {
	// Update regex to handle flexible whitespace
	re := regexp.MustCompile(`account sequence mismatch,\s*expected\s+(\d+),\s*got\s+(\d+)`)
	matches := re.FindStringSubmatch(errorMessage)

	if len(matches) == 3 {
		expected, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil {
			return 0, 0, err
		}

		current, err := strconv.ParseUint(matches[2], 10, 64)
		if err != nil {
			return 0, 0, err
		}

		return expected, current, nil
	}
	return 0, 0, fmt.Errorf("sequence numbers not found in error message")
}

// Extract gasWanted and gasUsed values from out of gas error message
func parseGasFromOutOfGasError(errorMessage string) (wanted uint64, used uint64, err error) {
	re := regexp.MustCompile(`gasWanted:\s*(\d+),\s*gasUsed:\s*(\d+)`)
	matches := re.FindStringSubmatch(errorMessage)

	if len(matches) == 3 {
		wanted, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil {
			return 0, 0, err
		}

		used, err := strconv.ParseUint(matches[2], 10, 64)
		if err != nil {
			return 0, 0, err
		}

		return wanted, used, nil
	}
	return 0, 0, fmt.Errorf("gas values not found in error message")
}

// Extract got and required fee values from insufficient fee error message
func parseInsufficientFeeError(errorMessage, denom string) (got uint64, required uint64, err error) {
	// Escape denom in case it contains special regex characters
	escapedDenom := regexp.QuoteMeta(denom)
	// Updated regex to handle the longer error format
	re := regexp.MustCompile(fmt.Sprintf(`got:\s*(\d+)%s\s*required:\s*(\d+)%s`, escapedDenom, escapedDenom))
	matches := re.FindStringSubmatch(errorMessage)

	if len(matches) == 3 {
		got, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse got fee: %w", err)
		}

		required, err := strconv.ParseUint(matches[2], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse required fee: %w", err)
		}

		return got, required, nil
	}
	return 0, 0, fmt.Errorf("fee values not found in error message")
}

// EstimateRequiredBaseGas calculates the required BaseGas for retry given the actual gasUsed and previous estimation
// excessCorrectionTimes determines how many times to apply the excess correction
func EstimateRequiredBaseGas(gasWanted, gasUsed, baseGas uint64, excessCorrectionTimes int64) uint64 {
	// If gasWanted <= baseGas, return the larger of baseGas and gasUsed
	if gasWanted <= baseGas {
		if gasUsed > baseGas {
			return gasUsed
		}
		return baseGas
	}

	// Calculate data gas estimate
	dataGasEstimate := gasWanted - baseGas

	// If gasUsed <= dataGasEstimate (shouldn't happen in out-of-gas scenarios)
	// return the larger value
	if gasUsed <= dataGasEstimate {
		if dataGasEstimate > baseGas {
			return dataGasEstimate
		}
		return baseGas
	}

	// Calculate new base gas
	newBaseGas := gasUsed - dataGasEstimate

	// Apply excess corrections
	newBaseGas += GAS_EXCESS_CORRECTION * uint64(excessCorrectionTimes) // nolint: gosec  // reason: small controlled value

	return newBaseGas
}

func parseAndSetNewWalletSequence(ctx context.Context, err error, node *NodeConfig, infoMsg string) (string, error) {
	connectionManager := node.ConnectionManager
	walletConfig, errorWalletConfig := connectionManager.GetWalletConfig()
	if errorWalletConfig != nil {
		return "", errorWalletConfig
	}

	expectedSeqNum, currentSeqNum, err := parseSequenceFromAccountMismatchError(err.Error())
	if err != nil {
		log.Error().Err(err).
			Str("rpc", node.ServerAddress).
			Str("msg", infoMsg).
			Msg("Failed to parse sequence from error - retrying with regular delay")
		if DoneOrWait(ctx, walletConfig.RetryDelay) {
			return ErrorProcessingError, ctx.Err()
		}
	}

	log.Info().
		Uint64("expected", expectedSeqNum).
		Uint64("current", currentSeqNum).
		Msg("Retrying resetting sequence from current to expected")

	wallet, err := connectionManager.GetWallet()
	if err != nil {
		return "", err
	}
	wallet.SetSequence(expectedSeqNum)

	if DoneOrWait(ctx, walletConfig.AccountSequenceRetryDelay) {
		return ErrorProcessingError, ctx.Err()
	}
	return ErrorProcessingResetSequence, nil
}

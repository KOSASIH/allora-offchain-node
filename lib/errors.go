package lib

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	errorsmod "cosmossdk.io/errors"
	emissions "github.com/allora-network/allora-chain/x/emissions/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/rs/zerolog/log"
	feemarkettypes "github.com/skip-mev/feemarket/x/feemarket/types"
)

// Error codes for the module
const ErrorCodespace = "allora-offchain-lib"

var (
	ErrHTTP              = errorsmod.Register(ErrorCodespace, 1, "http error")
	ErrNotEnoughBalance  = errorsmod.Register(ErrorCodespace, 2, "not enough balance")
	ErrNotRegistered     = errorsmod.Register(ErrorCodespace, 3, "not registered")
	ErrStakeBelowMin     = errorsmod.Register(ErrorCodespace, 4, "stake below minimum")
	ErrFullMempool       = errorsmod.Register(ErrorCodespace, 5, "full mempool")
	ErrReadPanic         = errorsmod.Register(ErrorCodespace, 6, "read panic")
	ErrConnectionRefused = errorsmod.Register(ErrorCodespace, 7, "connection refused")
	ErrUnexpectedError   = errorsmod.Register(ErrorCodespace, 10, "unexpected error")
)

// Marker for ABCI error codes
const ErrorMessageAbciErrorCodeMarker = "error code:"

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

// Excess correction in gas for txs that are not successful
const ExcessCorrectionInGas = 20000

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
const ErrorProcessingError = "error"
const ErrorProcessingFailure = "failure"
const ErrorProcessingSwitchingNode = "switch"

// HTTP status codes that trigger node switching
var HTTPStatusCodeCodesSwitchingNode = map[int]bool{
	403: true, // Forbidden
	429: true, // Too Many Requests
	502: true, // Bad Gateway
	503: true, // Service Unavailable
	504: true, // Gateway Timeout
	505: true, // HTTP Version Not Supported
}

// calculateExponentialBackoffDelay returns a duration based on retry count and base delay
func calculateExponentialBackoffDelaySeconds(baseDelay int64, retryCount int64) int64 {
	return int64(math.Pow(float64(baseDelay), float64(retryCount)))
}

// processError handles the error messages.
func ProcessErrorTx(ctx context.Context, err error, infoMsg string, retryCount, retryMax int64, node *NodeConfig) (string, error) {
	if strings.Contains(err.Error(), ErrorMessageAbciErrorCodeMarker) {
		re := regexp.MustCompile(`error code: '(\d+)'`)
		matches := re.FindStringSubmatch(err.Error())
		if len(matches) == 2 {
			errorCode, parseErr := strconv.ParseUint(matches[1], 10, 32)
			if parseErr != nil {
				log.Error().Err(parseErr).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Failed to parse ABCI error code, skipping ABCI error code triage")
			} else {
				if errorCode > math.MaxUint32 {
					log.Error().Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Parsed ABCI error code exceeds uint32 bounds, skipping ABCI error code triage")
				} else {
					return triageABCIErrorCode(ctx, uint32(errorCode), err, infoMsg, retryCount, retryMax, node) //nolint:gosec // Safe conversion - we check bounds above
				}
			}
		} else {
			log.Warn().Str("msg", infoMsg).Msg("Unmatched error format, cannot classify as ABCI error")
		}
	}

	// Check if error is HTTP status code
	if processingType, err := triageHTTPStatusError(err, node, infoMsg); err != nil {
		return processingType, err
	}

	return triageStringMatchingError(ctx, err, infoMsg, node)
}

// triageABCIErrorCode handles specific ABCI error codes and returns appropriate processing instructions
func triageABCIErrorCode(ctx context.Context, errorCode uint32, err error, infoMsg string, retryCount, retryMax int64, node *NodeConfig) (string, error) {
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
			delay := calculateExponentialBackoffDelaySeconds(node.Wallet.RetryDelay, retryCount)
			if DoneOrWait(ctx, delay) {
				return ErrorProcessingError, ctx.Err()
			}
			log.Info().
				Err(err).
				Str("msg", infoMsg).
				Msg("Mempool is full, retrying with exponential backoff")
			return ErrorProcessingContinue, nil
		}
	case sdkerrors.ErrWrongSequence.ABCICode(), sdkerrors.ErrInvalidSequence.ABCICode():
		log.Warn().
			Err(err).
			Str("msg", infoMsg).
			Int64("delay", node.Wallet.AccountSequenceRetryDelay).
			Msg("Account sequence mismatch detected, retrying with fixed delay")
		// Wait a fixed block-related waiting time
		if DoneOrWait(ctx, node.Wallet.AccountSequenceRetryDelay) {
			return ErrorProcessingError, ctx.Err()
		}
		return ErrorProcessingContinue, nil
	case sdkerrors.ErrInsufficientFee.ABCICode():
		log.Info().
			Err(err).
			Str("msg", infoMsg).
			Msg("Insufficient fees")
		return ErrorProcessingFees, nil
	case feemarkettypes.ErrNoFeeCoins.ABCICode():
		log.Info().
			Err(err).
			Str("msg", infoMsg).
			Msg("No fee coins")
		return ErrorProcessingFees, nil
	case sdkerrors.ErrTxTooLarge.ABCICode():
		return ErrorProcessingError, errorsmod.Wrapf(err, "tx too large")
	case sdkerrors.ErrTxInMempoolCache.ABCICode():
		return ErrorProcessingError, errorsmod.Wrapf(err, "tx already in mempool cache")
	case sdkerrors.ErrInvalidChainID.ABCICode():
		return ErrorProcessingError, errorsmod.Wrapf(err, "invalid chain-id")
	case sdkerrors.ErrTxTimeoutHeight.ABCICode():
		return ErrorProcessingFailure, errorsmod.Wrapf(err, "tx timeout height")
	case emissions.ErrWorkerNonceWindowNotAvailable.ABCICode():
		log.Warn().
			Err(err).
			Str("msg", infoMsg).
			Msg("Worker window not available, retrying with exponential backoff")
		delay := calculateExponentialBackoffDelaySeconds(node.Wallet.RetryDelay, retryCount)
		if DoneOrWait(ctx, delay) {
			return ErrorProcessingError, ctx.Err()
		}
		return ErrorProcessingContinue, nil
	case emissions.ErrReputerNonceWindowNotAvailable.ABCICode():
		log.Warn().
			Err(err).
			Str("msg", infoMsg).
			Msg("Reputer window not available, retrying with exponential backoff")
		delay := calculateExponentialBackoffDelaySeconds(node.Wallet.RetryDelay, retryCount)
		if DoneOrWait(ctx, delay) {
			return ErrorProcessingError, ctx.Err()
		}
		return ErrorProcessingContinue, nil
	default:
		log.Info().Uint32("errorCode", errorCode).Str("msg", infoMsg).Msg("ABCI error, but not special case - regular retry")
		return ErrorProcessingError, err
	}
}

// Triages error by string matching
func triageStringMatchingError(ctx context.Context, err error, infoMsg string, node *NodeConfig) (string, error) {
	if strings.Contains(err.Error(), ErrorMessageAccountSequenceMismatch) {
		log.Warn().
			Err(err).
			Str("rpc", node.RPC).
			Str("msg", infoMsg).
			Int64("delay", node.Wallet.AccountSequenceRetryDelay).
			Msg("Account sequence mismatch detected, re-fetching sequence")
		if DoneOrWait(ctx, node.Wallet.AccountSequenceRetryDelay) {
			return ErrorProcessingError, ctx.Err()
		}
		return ErrorProcessingContinue, nil
	} else if strings.Contains(err.Error(), ErrorContextDeadlineExceeded) {
		log.Warn().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Context deadline exceeded, switching to next node")
		return ErrorProcessingSwitchingNode, err
	} else if strings.Contains(err.Error(), ErrorMessageWaitingForNextBlock) {
		log.Warn().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Tx accepted in mempool, it will be included in the following block(s) - not retrying")
		return ErrorProcessingOk, nil
	} else if strings.Contains(err.Error(), ErrorMessageDataAlreadySubmitted) || strings.Contains(err.Error(), ErrorMessageCannotUpdateEma) {
		log.Warn().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Already submitted data for this epoch.")
		return ErrorProcessingOk, nil
	} else if strings.Contains(err.Error(), ErrorMessageTimeoutHeight) {
		log.Warn().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Tx failed because of timeout height")
		return ErrorProcessingFailure, err
	} else if strings.Contains(err.Error(), ErrorMessageNotPermittedToSubmitPayload) {
		log.Warn().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Actor is not permitted to submit payload")
		return ErrorProcessingFailure, err
	} else if strings.Contains(err.Error(), ErrorMessageNoInferencesFoundForTopic) {
		log.Warn().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("No inferences found for topic")
		return ErrorProcessingFailure, err
	} else if strings.Contains(err.Error(), ErrorMessageNotPermittedToAddStake) {
		log.Warn().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Actor is not permitted to add stake")
		return ErrorProcessingFailure, err
	} else if strings.Contains(err.Error(), ErrorMessageReadFlatPanic) || strings.Contains(err.Error(), ErrorMessageReadPerBytePanic) {
		log.Warn().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Read panic, switching to next node")
		return ErrorProcessingSwitchingNode, ErrReadPanic
	} else if strings.Contains(err.Error(), ErrorMessageConnectionRefused) {
		log.Warn().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Connection refused, switching to next node")
		return ErrorProcessingSwitchingNode, ErrConnectionRefused
	}
	log.Info().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Unknown error")
	return ErrorProcessingError, errorsmod.Wrap(ErrUnexpectedError, err.Error())
}

// triageHTTPStatusError checks if the error contains an HTTP status code and determines if node switching is needed
func triageHTTPStatusError(err error, node *NodeConfig, infoMsg string) (string, error) {
	statusCode, statusMessage, parseErr := ParseHTTPStatus(err.Error())
	if parseErr == nil {
		log.Warn().
			Int("statusCode", statusCode).
			Str("statusMessage", statusMessage).
			Str("msg", infoMsg).
			Msg("HTTP status code detected")

		// When status code is in the list of codes that trigger node switching, switch to next node without retries
		if HTTPStatusCodeCodesSwitchingNode[statusCode] {
			log.Warn().
				Str("rpc", node.RPC).
				Int("statusCode", statusCode).
				Str("statusMessage", statusMessage).
				Str("msg", infoMsg).
				Msg("HTTP status error code detected, switching to next node")
			return ErrorProcessingSwitchingNode, ErrHTTP
		}
	}
	return "", nil
}

// ParseStatus parses a status code and message from a given text string.
func ParseHTTPStatus(input string) (int, string, error) {
	// Regular expression to match "Status: <code> <message>" or similar patterns in text
	re := regexp.MustCompile(`(?i)status:\s*(\d+)\s*([^,]*)`)

	matches := re.FindStringSubmatch(input)
	if len(matches) < 3 {
		return 0, "", errors.New("invalid input format")
	}

	// Parse the status code
	statusCode, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, "", fmt.Errorf("invalid status code: %v", err)
	}

	// Get the status message
	statusMessage := strings.TrimSpace(matches[2])

	return statusCode, statusMessage, nil
}

// Returns true if the error is a switching-node error
func IsErrorSwitchingNode(err error) bool {
	return errors.Is(err, ErrHTTP) ||
		errors.Is(err, ErrFullMempool) ||
		errors.Is(err, ErrReadPanic) ||
		errors.Is(err, ErrConnectionRefused) ||
		errors.Is(err, ErrUnexpectedError)
}

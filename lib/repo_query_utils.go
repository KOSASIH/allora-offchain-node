package lib

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	"github.com/rs/zerolog/log"

	"github.com/cosmos/cosmos-sdk/types/query"
)

// QueryDataWithRetry attempts to query data with a uniform backoff strategy for retries.
func QueryDataWithRetry[T any](
	ctx context.Context,
	maxRetries int64,
	delaySeconds int64,
	queryFunc func(context.Context, query.PageRequest) (T, error),
	req query.PageRequest,
	infoMsg string,
	node *NodeConfig,
) (T, error) {
	var result T

	walletConfig, err := node.ConnectionManager.GetWalletConfig()
	if err != nil {
		return result, errorsmod.Wrapf(err, "Error getting wallet config")
	}

	for retryCount := int64(0); retryCount <= maxRetries; retryCount++ {
		log.Trace().Msgf("QueryDataWithRetry iteration started (%d/%d): %s", retryCount, maxRetries, infoMsg)
		result, err = queryFunc(ctx, req)
		if err == nil {
			return result, nil
		}

		// Log the error for each retry.
		log.Error().Err(err).Msgf("Query failed, retrying... (Retry %d/%d): %s", retryCount, maxRetries, infoMsg)

		errorResponse, err := ProcessErrorTx(ctx, err, infoMsg, retryCount, walletConfig.MaxRetries, node)
		switch errorResponse {
		case ErrorProcessingOk:
			return result, nil
		case ErrorProcessingError:
			// if error has not been handled, sleep and retry with regular delay
			if err != nil {
				log.Error().Err(err).Str("rpc", node.ServerAddress).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, walletConfig.MaxRetries)
				// Wait for the uniform delay before retrying
				if DoneOrWait(ctx, walletConfig.RetryDelay) {
					return result, ctx.Err()
				}
				continue
			}
		case ErrorProcessingContinue:
			// Error has not been handled, just continue next iteration
			continue
		case ErrorProcessingFees:
			log.Debug().Msg("Query failed due to fees limit")
			return result, err
		case ErrorProcessingGas:
			log.Debug().Msg("Query failed due to gas limit")
			return result, err
		case ErrorProcessingFailure:
			return result, errorsmod.Wrapf(err, "query failed and not retried")
		case ErrorProcessingSwitchingNode:
			return result, err
		default:
			return result, errorsmod.Wrapf(err, "failed to process error")
		}

		// Wait for the uniform delay before retrying
		if DoneOrWait(ctx, delaySeconds) {
			break
		}
	}

	// All retries failed, return the last error
	return result, err
}

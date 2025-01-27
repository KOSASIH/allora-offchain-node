package lib

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	errorsmod "cosmossdk.io/errors"
	cosmossdk_io_math "cosmossdk.io/math"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog/log"
)

// SendDataWithRetry attempts to send data, handling retries, with fee awareness.
// Custom handling for different errors.
func (node *NodeConfig) SendDataWithRetry(ctx context.Context, req sdktypes.Msg, infoMsg string, timeoutHeight uint64) (*cosmosclient.Response, error) {
	// Excess fees correction factor translated to fees using configured gas prices
	// This value is updated by the fee price update routine - making copy for consistency within method
	gasPrices := GetGasPrice()

	excessFactorFees := float64(ExcessCorrectionInGas) * gasPrices
	// Keep track of how many times fees need to be recalculated to avoid missing fee info between errors
	recalculateFees := 1
	// Use to keep track of expected sequence number between errors
	globalExpectedSeqNum := uint64(0)

	// Create tx options with timeout height if specified
	if timeoutHeight > 0 {
		log.Debug().Str("rpc", node.RPC).Uint64("timeoutHeight", timeoutHeight).Msg("Setting timeout height for tx")
		node.Chain.Client.TxFactory = node.Chain.Client.TxFactory.WithTimeoutHeight(timeoutHeight)
	}

	for retryCount := int64(0); retryCount <= node.Wallet.MaxRetries; retryCount++ {
		log.Debug().Msgf("SendDataWithRetry iteration started (%d/%d)", retryCount, node.Wallet.MaxRetries)
		// Create tx without fees to simulate tx creation and get estimated gas and seq number
		txOptions := cosmosclient.TxOptions{} // nolint: exhaustruct
		if globalExpectedSeqNum > 0 && node.Chain.Client.TxFactory.Sequence() != globalExpectedSeqNum {
			log.Debug().
				Str("rpc", node.RPC).
				Uint64("expected", globalExpectedSeqNum).
				Uint64("current", node.Chain.Client.TxFactory.Sequence()).
				Msg("Resetting sequence to expected from previous sequence errors")
			node.Chain.Client.TxFactory = node.Chain.Client.TxFactory.WithSequence(globalExpectedSeqNum)
		}
		txService, err := node.Chain.Client.CreateTxWithOptions(ctx, node.Chain.Account, txOptions, req)
		if err != nil {
			// Handle error on creation of tx, before broadcasting
			if strings.Contains(err.Error(), ErrorMessageAccountSequenceMismatch) {
				log.Warn().Err(err).Str("msg", infoMsg).Msg("Account sequence mismatch detected, resetting sequence")
				expectedSeqNum, currentSeqNum, err := parseSequenceFromAccountMismatchError(err.Error())
				if err != nil {
					log.Error().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Failed to parse sequence from error - retrying with regular delay")
					if DoneOrWait(ctx, node.Wallet.RetryDelay) {
						return nil, ctx.Err()
					}
					continue
				}
				// Reset sequence to expected in the client's tx factory
				node.Chain.Client.TxFactory = node.Chain.Client.TxFactory.WithSequence(expectedSeqNum)
				log.Info().Uint64("expected", expectedSeqNum).Uint64("current", currentSeqNum).Msg("Retrying resetting sequence from current to expected")
				txService, err = node.Chain.Client.CreateTxWithOptions(ctx, node.Chain.Account, txOptions, req)
				if err != nil {
					log.Error().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msg("Failed to reset sequence second time, retrying with regular delay")
					if DoneOrWait(ctx, node.Wallet.RetryDelay) {
						return nil, ctx.Err()
					}
					continue
				}
				// if creation is successful, make the expected sequence number persistent
				globalExpectedSeqNum = expectedSeqNum
			} else {
				errorResponse, err := ProcessErrorTx(ctx, err, infoMsg, retryCount, node.Wallet.MaxRetries, node)
				switch errorResponse {
				case ErrorProcessingOk:
					return &cosmosclient.Response{}, nil // nolint: exhaustruct
				case ErrorProcessingError:
					// if error has not been handled, sleep and retry with regular delay
					if err != nil {
						log.Error().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, node.Wallet.MaxRetries)
						// Wait for the uniform delay before retrying
						if DoneOrWait(ctx, node.Wallet.RetryDelay) {
							return nil, ctx.Err()
						}
						continue
					}
				case ErrorProcessingContinue:
					// Error has not been handled, just continue next iteration
					continue
				case ErrorProcessingFees:
					// Error has not been handled, just mark as recalculate fees on this iteration
					log.Debug().Msg("Marking fee recalculation on tx creation")
				case ErrorProcessingFailure:
					return nil, errorsmod.Wrapf(err, "tx failed and not retried")
				case ErrorProcessingSwitchingNode:
					return nil, err
				default:
					return nil, errorsmod.Wrapf(err, "failed to process error")
				}
			}
		} else {
			log.Trace().Msg("Create tx with account sequence OK")
		}

		// Handle fees if necessary
		if gasPrices > 0 {

			// Precalculate fees
			estimatedGas := float64(txService.Gas()) * node.Wallet.GasAdjustment
			feesFloat := float64(estimatedGas+ExcessCorrectionInGas) * gasPrices
			fees := cosmossdk_io_math.NewInt(int64(feesFloat))
			// Add excess fees correction factor to increase with each fee-problematic retry
			feeAdjustment := int64(float64(recalculateFees) * excessFactorFees)
			fees = fees.Add(cosmossdk_io_math.NewInt(feeAdjustment))
			// Limit fees to maxFees
			if fees.GT(node.Wallet.MaxFees.Number) {
				log.Warn().Uint64("gas", txService.Gas()).Interface("limit", node.Wallet.MaxFees).Msg("Gas limit exceeded, using maxFees instead")
				fees = node.Wallet.MaxFees.Number
			}
			txOptions := cosmosclient.TxOptions{ // nolint: exhaustruct
				Fees: fmt.Sprintf("%suallo", fees.String()),
			}
			log.Info().Str("fees", txOptions.Fees).Msg("Attempting tx with calculated fees")
			txService, err = node.Chain.Client.CreateTxWithOptions(ctx, node.Chain.Account, txOptions, req)
			if err != nil {
				log.Error().Err(err).Str("fees", txOptions.Fees).Msg("Failed to create tx with calculated fees")
				return nil, err
			}
			log.Info().Str("fees", txOptions.Fees).Msg("Successfully created tx with calculated fees")
		}

		log.Info().Msg("Creation of tx successful, broadcasting tx")
		// Broadcast tx
		txResponse, err := txService.Broadcast(ctx)
		if err == nil {
			log.Info().Str("rpc", node.RPC).Str("msg", infoMsg).Str("txHash", txResponse.TxHash).Msg("Success")
			return &txResponse, nil
		}

		// Handle error on broadcasting
		errorResponse, err := ProcessErrorTx(ctx, err, infoMsg, retryCount, node.Wallet.MaxRetries, node)
		switch errorResponse {
		case ErrorProcessingOk:
			return &txResponse, nil
		case ErrorProcessingError:
			// Error has not been handled, sleep and retry with regular delay
			if err != nil {
				log.Error().Err(err).Str("rpc", node.RPC).Str("msg", infoMsg).Msgf("Failed, retrying... (Retry %d/%d)", retryCount, node.Wallet.MaxRetries)
				// Wait for the uniform delay before retrying
				if DoneOrWait(ctx, node.Wallet.RetryDelay) {
					return nil, ctx.Err()
				}
				continue
			}
		case ErrorProcessingContinue:
			// Error has not been handled, just continue next iteration
			continue
		case ErrorProcessingFees:
			// Error has not been handled, just mark as recalculate fees on this iteration
			log.Info().Msg("Insufficient fees, marking fee recalculation on tx broadcasting for retrial")
			recalculateFees += 1
			continue
		case ErrorProcessingFailure:
			return nil, errorsmod.Wrapf(err, "tx failed and not retried")
		case ErrorProcessingSwitchingNode:
			return nil, err
		default:
			return nil, errorsmod.Wrapf(err, "failed to process error")
		}
	}

	return nil, errorsmod.Wrapf(ErrUnexpectedError, "Tx failed after max retries")
}

// Extract expected and current sequence numbers from the error message
func parseSequenceFromAccountMismatchError(errorMessage string) (uint64, uint64, error) {
	re := regexp.MustCompile(`account sequence mismatch, expected (\d+), got (\d+)`)
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

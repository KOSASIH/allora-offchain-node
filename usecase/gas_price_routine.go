package usecase

import (
	"context"
	"time"

	"allora_offchain_node/lib"

	"github.com/rs/zerolog/log"
)

// UpdateGasPriceRoutine continuously updates the gas price at a specified interval
func (suite *UseCaseSuite) UpdateGasPriceRoutine(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Updating fee price routine: terminating.")
			return
		default:
			price, err := WithTimeoutResult(ctx, time.Duration(suite.Node.Wallet.TimeoutRPCSecondsQuery)*time.Second,
				func(ctx context.Context) (float64, error) {
					return suite.Node.GetBaseFee(ctx)
				})
			if err != nil {
				log.Error().Err(err).Msg("Error updating gas prices")
			}
			lib.SetGasPrice(price)
			log.Debug().Float64("gasPrice", lib.GetGasPrice()).Msg("Updating fee price routine: updating value.")
			time.Sleep(time.Duration(suite.Node.Wallet.GasPriceUpdateInterval) * time.Second)
		}
	}
}

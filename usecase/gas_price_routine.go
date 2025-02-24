package usecase

import (
	"context"
	"fmt"
	"time"

	"allora_offchain_node/lib"

	"github.com/rs/zerolog/log"
)

func (suite *UseCaseSuite) UpdateGasPrice(ctx context.Context, wallet *lib.Wallet, walletConfig *lib.WalletConfig) error {
	price, err := lib.RunWithNodeRetry(
		ctx,
		suite.ConnectionManager,
		func(node *lib.NodeConfig) (float64, error) {
			return WithTimeoutResult(ctx,
				time.Duration(walletConfig.TimeoutRPCSecondsQuery)*time.Second,
				func(ctx context.Context) (float64, error) {
					node, err := suite.ConnectionManager.GetCurrentQueryNode()
					if err != nil {
						return 0, fmt.Errorf("failed to get current query node: %w", err)
					}
					return node.GetBaseFee(ctx, wallet.GetDefaultBondDenom())
				})
		},
		"get base fee",
		lib.GRPC_MODE,
	)
	if err != nil {
		log.Error().Err(err).Msg("Error updating gas prices")
		return err
	}
	lib.SetGasPrice(price)
	return nil
}

// UpdateGasPriceRoutine continuously updates the gas price at a specified interval
func (suite *UseCaseSuite) UpdateGasPriceRoutine(ctx context.Context, wallet *lib.Wallet, walletConfig *lib.WalletConfig) {
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Updating fee price routine: terminating.")
			return
		default:
			err := suite.UpdateGasPrice(ctx, wallet, walletConfig)
			if err != nil {
				log.Error().Err(err).Msg("Error updating gas prices")
			}

			log.Debug().Float64("gasPrice", lib.GetGasPrice()).Msg("Updating fee price routine: updating value.")

			if lib.DoneOrWait(ctx, walletConfig.GasPriceUpdateInterval) {
				log.Error().Msg("Updating fee price routine: terminating.")
				return
			}
		}
	}
}

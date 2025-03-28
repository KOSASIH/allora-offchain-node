package usecase

import (
	lib "allora_offchain_node/lib"
	auth "allora_offchain_node/lib/auth"
	metrics "allora_offchain_node/metrics"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	alloraMath "github.com/allora-network/allora-chain/math"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/rs/zerolog/log"
)

// Get the reputer's values at the block from the chain
// Compute loss bundle with the reputer provided Loss function and ground truth
// sign and commit to chain
func (suite *UseCaseSuite) BuildCommitReputerPayload(ctx context.Context, reputer lib.ReputerConfig, nonce lib.BlockHeight, timeoutHeight uint64) error {
	log := log.With().Uint64("topicId", reputer.TopicId).Str("actorType", "reputer").Logger()
	log.Info().Msg("Building reputer payload")
	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		return errorsmod.Wrapf(err, "Error getting wallet")
	}
	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		return errorsmod.Wrapf(err, "Error getting wallet config")
	}

	valueBundle, err := lib.RunWithNodeRetry(
		ctx,
		suite.ConnectionManager,
		func(node *lib.NodeConfig) (*emissionstypes.ValueBundle, error) {
			return node.GetReputerValuesAtBlock(ctx, reputer.TopicId, nonce)
		},
		"get reputer values",
		lib.GRPC_MODE,
	)
	if err != nil {
		return errorsmod.Wrapf(err, "error getting reputer values, topic: %d, blockHeight: %d", reputer.TopicId, nonce)
	}
	valueBundle.ReputerRequestNonce = &emissionstypes.ReputerRequestNonce{
		ReputerNonce: &emissionstypes.Nonce{BlockHeight: nonce},
	}
	valueBundle.Reputer = wallet.Address

	sourceTruth, err := reputer.GroundTruthEntrypoint.GroundTruth(reputer, nonce)
	if err != nil {
		return errorsmod.Wrapf(err, "error getting source truth from reputer, topicId: %d, blockHeight: %d", reputer.TopicId, nonce)
	}
	suite.Metrics.IncrementMetricsCounter(metrics.TruthRequestCount, wallet.Address, reputer.TopicId)

	lossBundle, err := suite.ComputeLossBundle(sourceTruth, valueBundle, reputer)
	if err != nil {
		return errorsmod.Wrapf(err, "error computing loss bundle, topic: %d, blockHeight: %d", reputer.TopicId, nonce)
	}
	suite.Metrics.IncrementMetricsCounter(metrics.ReputerDataBuildCount, wallet.Address, reputer.TopicId)

	signedValueBundle, err := suite.SignReputerValueBundle(&lossBundle)
	if err != nil {
		return errorsmod.Wrapf(err, "error signing reputer value bundle, topic: %d, blockHeight: %d", reputer.TopicId, nonce)
	}

	if err := signedValueBundle.Validate(); err != nil {
		return errorsmod.Wrapf(err, "error validating reputer value bundle, topic: %d, blockHeight: %d", reputer.TopicId, nonce)
	}

	req := &emissionstypes.InsertReputerPayloadRequest{
		Sender:             wallet.Address,
		ReputerValueBundle: signedValueBundle,
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		log.Error().Err(err).Msgf("Error marshaling MsgInserReputerPayload to print Msg as JSON")
	} else {
		log.Debug().Msgf("Sending InsertReputerPayload to chain %s", string(reqJSON))
	}

	if walletConfig.SubmitTx {
		_, err = suite.ConnectionManager.SendDataWithNodeRetry(ctx, req, timeoutHeight, "Send Reputer Data to chain")
		if err != nil {
			return errorsmod.Wrapf(err, "error sending Reputer Data to chain, topic: %d, blockHeight: %d", reputer.TopicId, nonce)
		}
		suite.Metrics.IncrementMetricsCounter(metrics.ReputerChainSubmissionCount, wallet.Address, reputer.TopicId)
	} else {
		log.Info().Msg("SubmitTx=false; Skipping sending Reputer Data to chain")
	}

	return nil
}

func (suite *UseCaseSuite) ComputeLossBundle(sourceTruth string, vb *emissionstypes.ValueBundle, reputer lib.ReputerConfig) (emissionstypes.InputValueBundle, error) {
	if vb == nil {
		return emissionstypes.InputValueBundle{}, errors.New("nil ValueBundle")
	}
	// Check if vb is empty
	if IsEmpty(*vb) {
		return emissionstypes.InputValueBundle{}, errors.New("empty ValueBundle")
	}
	if err := emissionstypes.ValidateDec(vb.CombinedValue); err != nil {
		return emissionstypes.InputValueBundle{}, errors.New("ValueBundle - invalid CombinedValue")
	}
	if err := emissionstypes.ValidateDec(vb.NaiveValue); err != nil {
		return emissionstypes.InputValueBundle{}, errors.New("ValueBundle - invalid NaiveValue")
	}

	lossMethodOptions := reputer.LossFunctionParameters.LossMethodOptions
	// Use the cached IsNeverNegative value
	isNeverNegative := false
	if reputer.LossFunctionParameters.IsNeverNegative != nil {
		isNeverNegative = *reputer.LossFunctionParameters.IsNeverNegative
	} else {
		var err error
		isNeverNegative, err = reputer.LossFunctionEntrypoint.IsLossFunctionNeverNegative(reputer, lossMethodOptions)
		if err != nil {
			return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "failed to determine if loss function is never negative")
		}
		// cache the result
		reputer.LossFunctionParameters.IsNeverNegative = &isNeverNegative
	}

	losses := emissionstypes.InputValueBundle{ // nolint: exhaustruct
		TopicId:             vb.TopicId,
		ReputerRequestNonce: vb.ReputerRequestNonce,
		Reputer:             vb.Reputer,
		ExtraData:           vb.ExtraData,
	}

	computeLoss := func(value alloraMath.Dec, description string) (alloraMath.Dec, error) {
		lossStr, err := reputer.LossFunctionEntrypoint.LossFunction(reputer, sourceTruth, value.String(), lossMethodOptions)
		if err != nil {
			return alloraMath.Dec{}, errorsmod.Wrapf(err, "error computing loss for %s", description)
		}

		loss, err := alloraMath.NewDecFromString(lossStr)
		if err != nil {
			return alloraMath.Dec{}, errorsmod.Wrapf(err, "error parsing loss value for %s", description)
		}

		if isNeverNegative {
			loss, err = alloraMath.Log10(loss)
			if err != nil {
				return alloraMath.Dec{}, errorsmod.Wrapf(err, "error Log10 for %s", description)
			}
		}

		if err := emissionstypes.ValidateDec(loss); err != nil {
			return alloraMath.Dec{}, errorsmod.Wrapf(err, "invalid loss value for %s", description)
		}

		return loss, nil
	}

	// Combined Value
	if combinedLoss, err := computeLoss(vb.CombinedValue, "combined value"); err != nil {
		return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error computing loss for combined value")
	} else {
		losses.CombinedValue, err = alloraMath.NewBoundedExp40Dec(combinedLoss)
		if err != nil {
			return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error converting combined loss to BoundedExp40Dec")
		}
	}

	// Naive Value
	if naiveLoss, err := computeLoss(vb.NaiveValue, "naive value"); err != nil {
		return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error computing loss for naive value")
	} else {
		losses.NaiveValue, err = alloraMath.NewBoundedExp40Dec(naiveLoss)
		if err != nil {
			return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error converting naive loss to BoundedExp40Dec")
		}
	}

	// Inferer Values
	losses.InfererValues = make([]*emissionstypes.InputWorkerAttributedValue, len(vb.InfererValues))
	for i, val := range vb.InfererValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("inferer value %d", i)); err != nil {
			return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error computing loss for inferer value")
		} else {
			boundedLoss, err := alloraMath.NewBoundedExp40Dec(loss)
			if err != nil {
				return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error converting naive loss to BoundedExp40Dec")
			}
			losses.InfererValues[i] = &emissionstypes.InputWorkerAttributedValue{Worker: val.Worker, Value: boundedLoss}
		}
	}

	// Forecaster Values
	losses.ForecasterValues = make([]*emissionstypes.InputWorkerAttributedValue, len(vb.ForecasterValues))
	for i, val := range vb.ForecasterValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("forecaster value %d", i)); err != nil {
			return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error computing loss for forecaster value")
		} else {
			boundedLoss, err := alloraMath.NewBoundedExp40Dec(loss)
			if err != nil {
				return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error converting naive loss to BoundedExp40Dec")
			}
			losses.ForecasterValues[i] = &emissionstypes.InputWorkerAttributedValue{Worker: val.Worker, Value: boundedLoss}
		}
	}

	// One Out Inferer Values
	losses.OneOutInfererValues = make([]*emissionstypes.InputWithheldWorkerAttributedValue, len(vb.OneOutInfererValues))
	for i, val := range vb.OneOutInfererValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("one out inferer value %d", i)); err != nil {
			return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error computing loss for one-out inferer value")
		} else {
			boundedLoss, err := alloraMath.NewBoundedExp40Dec(loss)
			if err != nil {
				return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error converting naive loss to BoundedExp40Dec")
			}
			losses.OneOutInfererValues[i] = &emissionstypes.InputWithheldWorkerAttributedValue{Worker: val.Worker, Value: boundedLoss}
		}
	}

	// One Out Forecaster Values
	losses.OneOutForecasterValues = make([]*emissionstypes.InputWithheldWorkerAttributedValue, len(vb.OneOutForecasterValues))
	for i, val := range vb.OneOutForecasterValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("one out forecaster value %d", i)); err != nil {
			return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error computing loss for one-out forecaster value")
		} else {
			boundedLoss, err := alloraMath.NewBoundedExp40Dec(loss)
			if err != nil {
				return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error converting naive loss to BoundedExp40Dec")
			}
			losses.OneOutForecasterValues[i] = &emissionstypes.InputWithheldWorkerAttributedValue{Worker: val.Worker, Value: boundedLoss}
		}
	}

	// One In Forecaster Values
	losses.OneInForecasterValues = make([]*emissionstypes.InputWorkerAttributedValue, len(vb.OneInForecasterValues))
	for i, val := range vb.OneInForecasterValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("one in forecaster value %d", i)); err != nil {
			return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error computing loss for one-in forecaster value")
		} else {
			boundedLoss, err := alloraMath.NewBoundedExp40Dec(loss)
			if err != nil {
				return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error converting naive loss to BoundedExp40Dec")
			}
			losses.OneInForecasterValues[i] = &emissionstypes.InputWorkerAttributedValue{Worker: val.Worker, Value: boundedLoss}
		}
	}

	losses.OneOutInfererForecasterValues = make([]*emissionstypes.InputOneOutInfererForecasterValues, len(vb.OneOutInfererForecasterValues))
	for i, val := range vb.OneOutInfererForecasterValues {
		oneOutInfererValues := make([]*emissionstypes.InputWithheldWorkerAttributedValue, len(val.OneOutInfererValues))
		for j, infererVal := range val.OneOutInfererValues {
			if loss, err := computeLoss(infererVal.Value, fmt.Sprintf("one out inferer value %d", j)); err != nil {
				return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error computing loss for one-out inferer value")
			} else {
				boundedLoss, err := alloraMath.NewBoundedExp40Dec(loss)
				if err != nil {
					return emissionstypes.InputValueBundle{}, errorsmod.Wrapf(err, "error converting naive loss to BoundedExp40Dec")
				}
				oneOutInfererValues[j] = &emissionstypes.InputWithheldWorkerAttributedValue{Worker: infererVal.Worker, Value: boundedLoss}
			}
		}

		losses.OneOutInfererForecasterValues[i] = &emissionstypes.InputOneOutInfererForecasterValues{
			Forecaster:          val.Forecaster,
			OneOutInfererValues: oneOutInfererValues,
		}
	}

	return losses, nil
}

func (suite *UseCaseSuite) SignReputerValueBundle(valueBundle *emissionstypes.InputValueBundle) (*emissionstypes.InputReputerValueBundle, error) {
	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		return &emissionstypes.InputReputerValueBundle{}, errorsmod.Wrapf(err, "error getting wallet") // nolint: exhaustruct
	}
	sig, pk, err := auth.MarshalAndSignByPrivKey(valueBundle, wallet.GetPrivKey(), wallet.AddressSDK)
	if err != nil {
		return &emissionstypes.InputReputerValueBundle{}, errorsmod.Wrapf(err, "error signing the InferenceForecastsBundle message") // nolint: exhaustruct
	}
	pkStr := hex.EncodeToString(pk)
	reputerValueBundle := &emissionstypes.InputReputerValueBundle{
		ValueBundle: valueBundle,
		Signature:   sig,
		Pubkey:      pkStr,
	}

	return reputerValueBundle, nil
}

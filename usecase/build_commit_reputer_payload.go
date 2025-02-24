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

func (suite *UseCaseSuite) ComputeLossBundle(sourceTruth string, vb *emissionstypes.ValueBundle, reputer lib.ReputerConfig) (emissionstypes.ValueBundle, error) {
	if vb == nil {
		return emissionstypes.ValueBundle{}, errors.New("nil ValueBundle")
	}
	// Check if vb is empty
	if IsEmpty(*vb) {
		return emissionstypes.ValueBundle{}, errors.New("empty ValueBundle")
	}
	if err := emissionstypes.ValidateDec(vb.CombinedValue); err != nil {
		return emissionstypes.ValueBundle{}, errors.New("ValueBundle - invalid CombinedValue")
	}
	if err := emissionstypes.ValidateDec(vb.NaiveValue); err != nil {
		return emissionstypes.ValueBundle{}, errors.New("ValueBundle - invalid NaiveValue")
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
			return emissionstypes.ValueBundle{}, errorsmod.Wrapf(err, "failed to determine if loss function is never negative")
		}
		// cache the result
		reputer.LossFunctionParameters.IsNeverNegative = &isNeverNegative
	}

	losses := emissionstypes.ValueBundle{ // nolint: exhaustruct
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
		return emissionstypes.ValueBundle{}, errorsmod.Wrapf(err, "error computing loss for combined value")
	} else {
		losses.CombinedValue = combinedLoss
	}

	// Naive Value
	if naiveLoss, err := computeLoss(vb.NaiveValue, "naive value"); err != nil {
		return emissionstypes.ValueBundle{}, errorsmod.Wrapf(err, "error computing loss for naive value")
	} else {
		losses.NaiveValue = naiveLoss
	}

	// Inferer Values
	losses.InfererValues = make([]*emissionstypes.WorkerAttributedValue, len(vb.InfererValues))
	for i, val := range vb.InfererValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("inferer value %d", i)); err != nil {
			return emissionstypes.ValueBundle{}, errorsmod.Wrapf(err, "error computing loss for inferer value")
		} else {
			losses.InfererValues[i] = &emissionstypes.WorkerAttributedValue{Worker: val.Worker, Value: loss}
		}
	}

	// Forecaster Values
	losses.ForecasterValues = make([]*emissionstypes.WorkerAttributedValue, len(vb.ForecasterValues))
	for i, val := range vb.ForecasterValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("forecaster value %d", i)); err != nil {
			return emissionstypes.ValueBundle{}, errorsmod.Wrapf(err, "error computing loss for forecaster value")
		} else {
			losses.ForecasterValues[i] = &emissionstypes.WorkerAttributedValue{Worker: val.Worker, Value: loss}
		}
	}

	// One Out Inferer Values
	losses.OneOutInfererValues = make([]*emissionstypes.WithheldWorkerAttributedValue, len(vb.OneOutInfererValues))
	for i, val := range vb.OneOutInfererValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("one out inferer value %d", i)); err != nil {
			return emissionstypes.ValueBundle{}, errorsmod.Wrapf(err, "error computing loss for one-out inferer value")
		} else {
			losses.OneOutInfererValues[i] = &emissionstypes.WithheldWorkerAttributedValue{Worker: val.Worker, Value: loss}
		}
	}

	// One Out Forecaster Values
	losses.OneOutForecasterValues = make([]*emissionstypes.WithheldWorkerAttributedValue, len(vb.OneOutForecasterValues))
	for i, val := range vb.OneOutForecasterValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("one out forecaster value %d", i)); err != nil {
			return emissionstypes.ValueBundle{}, errorsmod.Wrapf(err, "error computing loss for one-out forecaster value")
		} else {
			losses.OneOutForecasterValues[i] = &emissionstypes.WithheldWorkerAttributedValue{Worker: val.Worker, Value: loss}
		}
	}

	// One In Forecaster Values
	losses.OneInForecasterValues = make([]*emissionstypes.WorkerAttributedValue, len(vb.OneInForecasterValues))
	for i, val := range vb.OneInForecasterValues {
		if loss, err := computeLoss(val.Value, fmt.Sprintf("one in forecaster value %d", i)); err != nil {
			return emissionstypes.ValueBundle{}, errorsmod.Wrapf(err, "error computing loss for one-in forecaster value")
		} else {
			losses.OneInForecasterValues[i] = &emissionstypes.WorkerAttributedValue{Worker: val.Worker, Value: loss}
		}
	}
	return losses, nil
}

func (suite *UseCaseSuite) SignReputerValueBundle(valueBundle *emissionstypes.ValueBundle) (*emissionstypes.ReputerValueBundle, error) {
	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		return &emissionstypes.ReputerValueBundle{}, errorsmod.Wrapf(err, "error getting wallet") // nolint: exhaustruct
	}
	sig, pk, err := auth.MarshalAndSignByPrivKey(valueBundle, wallet.GetPrivKey(), wallet.AddressSDK)
	if err != nil {
		return &emissionstypes.ReputerValueBundle{}, errorsmod.Wrapf(err, "error signing the InferenceForecastsBundle message") // nolint: exhaustruct
	}
	pkStr := hex.EncodeToString(pk)
	reputerValueBundle := &emissionstypes.ReputerValueBundle{
		ValueBundle: valueBundle,
		Signature:   sig,
		Pubkey:      pkStr,
	}

	return reputerValueBundle, nil
}

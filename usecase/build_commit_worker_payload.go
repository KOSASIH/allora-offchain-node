package usecase

import (
	"allora_offchain_node/lib"
	auth "allora_offchain_node/lib/auth"
	"allora_offchain_node/metrics"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"

	errorsmod "cosmossdk.io/errors"
	"github.com/rs/zerolog/log"

	alloraMath "github.com/allora-network/allora-chain/math"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
)

func (suite *UseCaseSuite) BuildCommitWorkerPayload(ctx context.Context, worker lib.WorkerConfig, nonce *emissionstypes.Nonce, timeoutHeight uint64) error {
	log := log.With().Uint64("topicId", worker.TopicId).Str("actorType", "worker").Logger()
	log.Info().Msg("Building worker payload")

	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		return errorsmod.Wrapf(err, "Error getting wallet")
	}
	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		return errorsmod.Wrapf(err, "Error getting wallet config")
	}

	if worker.InferenceEntrypoint == nil && worker.ForecastEntrypoint == nil {
		return errors.New("Worker has no valid Inference or Forecast entrypoints")
	}

	var workerResponse = lib.WorkerResponse{ // nolint: exhaustruct
		WorkerConfig: worker,
	}

	if worker.InferenceEntrypoint != nil {
		inference, err := worker.InferenceEntrypoint.CalcInference(worker, nonce.BlockHeight)
		if err != nil {
			return errorsmod.Wrapf(err, "Error computing inference for worker, topicId: %d, blockHeight: %d", worker.TopicId, nonce.BlockHeight)
		}
		workerResponse.InfererValue = inference
		suite.Metrics.IncrementMetricsCounter(metrics.InferenceRequestCount, wallet.Address, worker.TopicId)
	}

	if worker.ForecastEntrypoint != nil {
		forecasts, err := worker.ForecastEntrypoint.CalcForecast(worker, nonce.BlockHeight)
		if err != nil {
			return errorsmod.Wrapf(err, "Error computing forecast for worker, topicId: %d, blockHeight: %d", worker.TopicId, nonce.BlockHeight)
		}
		workerResponse.ForecasterValues = forecasts
		suite.Metrics.IncrementMetricsCounter(metrics.ForecastRequestCount, wallet.Address, worker.TopicId)
	}

	workerPayload, err := suite.BuildWorkerPayload(workerResponse, nonce.BlockHeight)
	if err != nil {
		return errorsmod.Wrapf(err, "Error building worker payload, topicId: %d, blockHeight: %d", worker.TopicId, nonce.BlockHeight)
	}

	workerDataBundle, err := suite.SignWorkerPayload(&workerPayload)
	if err != nil {
		return errorsmod.Wrapf(err, "Error signing worker payload, topicId: %d, blockHeight: %d", worker.TopicId, nonce.BlockHeight)
	}
	workerDataBundle.Nonce = nonce
	workerDataBundle.TopicId = worker.TopicId

	if err := workerDataBundle.Validate(); err != nil {
		return errorsmod.Wrapf(err, "Error validating worker data bundle, topicId: %d, blockHeight: %d", worker.TopicId, nonce.BlockHeight)
	}

	req := &emissionstypes.InsertWorkerPayloadRequest{
		Sender:           wallet.Address,
		WorkerDataBundle: workerDataBundle,
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		log.Warn().Err(err).Msg("Error marshaling InsertWorkerPayload to print Msg as JSON")
	} else {
		log.Info().Str("req", string(reqJSON)).Msg("Sending InsertWorkerPayload to chain")
	}

	if walletConfig.SubmitTx {
		_, err = suite.ConnectionManager.SendDataWithNodeRetry(ctx, req, timeoutHeight, "Send Worker Data to chain")
		if err != nil {
			return errorsmod.Wrapf(err, "Error sending Worker Data to chain, topicId: %d, blockHeight: %d", worker.TopicId, nonce.BlockHeight)
		}
		suite.Metrics.IncrementMetricsCounter(metrics.WorkerChainSubmissionCount, wallet.Address, worker.TopicId)
	} else {
		log.Info().Msg("SubmitTx=false; Skipping sending Worker Data to chain")
	}
	return nil
}

func (suite *UseCaseSuite) BuildWorkerPayload(workerResponse lib.WorkerResponse, nonce emissionstypes.BlockHeight) (emissionstypes.InputInferenceForecastBundle, error) {
	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		return emissionstypes.InputInferenceForecastBundle{}, errorsmod.Wrapf(err, "error getting wallet") // nolint: exhaustruct
	}

	inferenceForecastsBundle := emissionstypes.InputInferenceForecastBundle{} // nolint: exhaustruct

	if workerResponse.InfererValue != "" {
		infererValue, err := alloraMath.NewBoundedExp40DecFromString(workerResponse.InfererValue)
		if err != nil {
			return emissionstypes.InputInferenceForecastBundle{}, errorsmod.Wrapf(err, "error converting infererValue to Dec") // nolint: exhaustruct
		}
		builtInference := &emissionstypes.InputInference{ // nolint: exhaustruct
			TopicId:     workerResponse.TopicId,
			Inferer:     wallet.Address,
			Value:       infererValue,
			BlockHeight: nonce,
		}
		inferenceForecastsBundle.Inference = builtInference
	}

	if len(workerResponse.ForecasterValues) > 0 {
		var forecasterElements []*emissionstypes.InputForecastElement // nolint: exhaustruct
		for _, val := range workerResponse.ForecasterValues {
			decVal, err := alloraMath.NewBoundedExp40DecFromString(val.Value)
			if err != nil {
				return emissionstypes.InputInferenceForecastBundle{}, errorsmod.Wrapf(err, "error converting forecasterValue to Dec") // nolint: exhaustruct
			}
			forecasterElements = append(forecasterElements, &emissionstypes.InputForecastElement{
				Inferer: val.Worker,
				Value:   decVal,
			})
		}

		if len(forecasterElements) > 0 {
			forecasterValues := &emissionstypes.InputForecast{ // nolint: exhaustruct
				TopicId:          workerResponse.TopicId,
				BlockHeight:      nonce,
				Forecaster:       wallet.Address,
				ForecastElements: forecasterElements,
				ExtraData:        nil,
			}
			inferenceForecastsBundle.Forecast = forecasterValues
		}
	}
	return inferenceForecastsBundle, nil
}

func (suite *UseCaseSuite) SignWorkerPayload(workerPayload *emissionstypes.InputInferenceForecastBundle) (*emissionstypes.InputWorkerDataBundle, error) {
	// Marshal and sign the bundle
	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		return &emissionstypes.InputWorkerDataBundle{}, errorsmod.Wrapf(err, "error getting wallet") // nolint: exhaustruct
	}
	sig, pk, err := auth.MarshalAndSignByPrivKey(workerPayload, wallet.GetPrivKey(), wallet.AddressSDK)
	if err != nil {
		return &emissionstypes.InputWorkerDataBundle{}, errorsmod.Wrapf(err, "error signing the InferenceForecastsBundle message") // nolint: exhaustruct
	}
	pkStr := hex.EncodeToString(pk)
	// Create workerDataBundle with signature
	workerDataBundle := &emissionstypes.InputWorkerDataBundle{ // nolint: exhaustruct
		Worker:                             wallet.Address,
		InferenceForecastsBundle:           workerPayload,
		InferencesForecastsBundleSignature: sig,
		Pubkey:                             pkStr,
	}

	return workerDataBundle, nil
}

package usecase

import (
	"allora_offchain_node/lib"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"

	errorsmod "cosmossdk.io/errors"
	"github.com/rs/zerolog/log"

	alloraMath "github.com/allora-network/allora-chain/math"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
)

func (suite *UseCaseSuite) BuildCommitWorkerPayload(ctx context.Context, worker lib.WorkerConfig, nonce *emissionstypes.Nonce, timeoutHeight uint64) error {
	log := log.With().Uint64("topicId", worker.TopicId).Str("actorType", "worker").Logger()
	log.Info().Msg("Building worker payload")

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
		suite.Metrics.IncrementMetricsCounter(lib.InferenceRequestCount, suite.RPCManager.GetCurrentNode().Chain.Address, worker.TopicId)
	}

	if worker.ForecastEntrypoint != nil {
		forecasts, err := worker.ForecastEntrypoint.CalcForecast(worker, nonce.BlockHeight)
		if err != nil {
			return errorsmod.Wrapf(err, "Error computing forecast for worker, topicId: %d, blockHeight: %d", worker.TopicId, nonce.BlockHeight)
		}
		workerResponse.ForecasterValues = forecasts
		suite.Metrics.IncrementMetricsCounter(lib.ForecastRequestCount, suite.RPCManager.GetCurrentNode().Chain.Address, worker.TopicId)
	}

	workerPayload, err := suite.BuildWorkerPayload(workerResponse, nonce.BlockHeight)
	if err != nil {
		return errorsmod.Wrapf(err, "Error building worker payload, topicId: %d, blockHeight: %d", worker.TopicId, nonce.BlockHeight)
	}
	suite.Metrics.IncrementMetricsCounter(lib.WorkerDataBuildCount, suite.RPCManager.GetCurrentNode().Chain.Address, worker.TopicId)

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
		Sender:           suite.RPCManager.GetCurrentNode().Wallet.Address,
		WorkerDataBundle: workerDataBundle,
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		log.Warn().Err(err).Msg("Error marshaling InsertWorkerPayload to print Msg as JSON")
	} else {
		log.Info().Str("req", string(reqJSON)).Msg("Sending InsertWorkerPayload to chain")
	}

	if suite.RPCManager.GetCurrentNode().Wallet.SubmitTx {
		_, err = suite.RPCManager.SendDataWithNodeRetry(ctx, req, timeoutHeight, "Send Worker Data to chain")
		if err != nil {
			return errorsmod.Wrapf(err, "Error sending Worker Data to chain, topicId: %d, blockHeight: %d", worker.TopicId, nonce.BlockHeight)
		}
		suite.Metrics.IncrementMetricsCounter(lib.WorkerChainSubmissionCount, suite.RPCManager.GetCurrentNode().Chain.Address, worker.TopicId)
	} else {
		log.Info().Msg("SubmitTx=false; Skipping sending Worker Data to chain")
	}
	return nil
}

func (suite *UseCaseSuite) BuildWorkerPayload(workerResponse lib.WorkerResponse, nonce emissionstypes.BlockHeight) (emissionstypes.InferenceForecastBundle, error) {

	inferenceForecastsBundle := emissionstypes.InferenceForecastBundle{} // nolint: exhaustruct

	if workerResponse.InfererValue != "" {
		infererValue, err := alloraMath.NewDecFromString(workerResponse.InfererValue)
		if err != nil {
			return emissionstypes.InferenceForecastBundle{}, errorsmod.Wrapf(err, "error converting infererValue to Dec") // nolint: exhaustruct
		}
		builtInference := &emissionstypes.Inference{ // nolint: exhaustruct
			TopicId:     workerResponse.TopicId,
			Inferer:     suite.RPCManager.GetCurrentNode().Wallet.Address,
			Value:       infererValue,
			BlockHeight: nonce,
		}
		inferenceForecastsBundle.Inference = builtInference
	}

	if len(workerResponse.ForecasterValues) > 0 {
		var forecasterElements []*emissionstypes.ForecastElement // nolint: exhaustruct
		for _, val := range workerResponse.ForecasterValues {
			decVal, err := alloraMath.NewDecFromString(val.Value)
			if err != nil {
				return emissionstypes.InferenceForecastBundle{}, errorsmod.Wrapf(err, "error converting forecasterValue to Dec") // nolint: exhaustruct
			}
			forecasterElements = append(forecasterElements, &emissionstypes.ForecastElement{
				Inferer: val.Worker,
				Value:   decVal,
			})
		}

		if len(forecasterElements) > 0 {
			forecasterValues := &emissionstypes.Forecast{ // nolint: exhaustruct
				TopicId:          workerResponse.TopicId,
				BlockHeight:      nonce,
				Forecaster:       suite.RPCManager.GetCurrentNode().Wallet.Address,
				ForecastElements: forecasterElements,
			}
			inferenceForecastsBundle.Forecast = forecasterValues
		}
	}
	return inferenceForecastsBundle, nil
}

func (suite *UseCaseSuite) SignWorkerPayload(workerPayload *emissionstypes.InferenceForecastBundle) (*emissionstypes.WorkerDataBundle, error) {
	// Marshall and sign the bundle
	protoBytesIn := make([]byte, 0) // Create a byte slice with initial length 0 and capacity greater than 0
	protoBytesIn, err := workerPayload.XXX_Marshal(protoBytesIn, true)
	if err != nil {
		return &emissionstypes.WorkerDataBundle{}, errorsmod.Wrapf(err, "error marshalling workerPayload") // nolint: exhaustruct
	}
	sig, pk, err := suite.RPCManager.GetCurrentNode().Chain.Client.Context().Keyring.Sign(suite.RPCManager.GetCurrentNode().Chain.Account.Name, protoBytesIn, signing.SignMode_SIGN_MODE_DIRECT)
	if err != nil {
		return &emissionstypes.WorkerDataBundle{}, errorsmod.Wrapf(err, "error signing the InferenceForecastsBundle message") // nolint: exhaustruct
	}
	pkStr := hex.EncodeToString(pk.Bytes())
	// Create workerDataBundle with signature
	workerDataBundle := &emissionstypes.WorkerDataBundle{ // nolint: exhaustruct
		Worker:                             suite.RPCManager.GetCurrentNode().Wallet.Address,
		InferenceForecastsBundle:           workerPayload,
		InferencesForecastsBundleSignature: sig,
		Pubkey:                             pkStr,
	}

	return workerDataBundle, nil
}

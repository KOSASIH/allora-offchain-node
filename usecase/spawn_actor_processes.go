package usecase

import (
	"allora_offchain_node/lib"
	"allora_offchain_node/metrics"
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	errorsmod "cosmossdk.io/errors"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/rand"
)

// Number of submission windows considered to be "near" the next window
// When time is near, the control is more accurate
const NUM_SUBMISSION_WINDOWS_FOR_SUBMISSION_NEARNESS int64 = 2

// Correction factor used when calculating time distances near window
// Waiting times under nearness circumstances are adjusted by this factor
const NEARNESS_CORRECTION_FACTOR float64 = 1.0

// Correction factor used when calculating time distances for new topics
const NEW_TOPIC_CORRECTION_FACTOR float64 = 0.5

// Minimum wait time between status checks
const WAIT_TIME_STATUS_CHECKS int64 = 2

// ActorProcessParams encapsulates the configuration needed for running actor processes
type ActorProcessParams[T lib.TopicActor] struct {
	// Configuration for the actor (Worker or Reputer)
	Config T
	// Function to process payloads (processWorkerPayload or processReputerPayload)
	ProcessPayload func(context.Context, T, int64, uint64) (int64, error)
	// Function to get nonces (GetLatestOpenWorkerNonceByTopicId or GetOldestReputerNonceByTopicId)
	GetNonce func(context.Context, emissionstypes.TopicId) (*emissionstypes.Nonce, error)
	// Window length used to determine when we're near submission time
	NearWindowLength int64
	// Actual submission window length
	SubmissionWindowLength int64
	// Actor type for logging ("worker" or "reputer")
	ActorType string
}

// launchGasRoutine initializes gas prices and starts the auto-update routine if needed
func (suite *UseCaseSuite) launchGasRoutine(ctx context.Context, walletConfig *lib.WalletConfig, wallet *lib.Wallet) error {
	// Initialize gas prices explicitly first
	err := suite.UpdateGasPrice(ctx, wallet, walletConfig)
	if err != nil {
		log.Error().Err(err).Msg("Error updating gas prices in auto mode - RPC availability issue?")
		return err
	}
	// After initialization, start auto-update routine
	go suite.UpdateGasPriceRoutine(ctx, wallet, walletConfig)
	return nil
}

// Spawns the actor processes and any associated non-essential routines
func (suite *UseCaseSuite) Start() error {

	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		log.Error().Err(err).Msg("Error getting wallet")
		return err
	}
	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		log.Error().Err(err).Msg("Error getting wallet config")
		return err
	}
	if walletConfig.GasPrices == lib.AutoGasPrices {
		if err := suite.launchGasRoutine(suite.nonEssentialCtx, walletConfig, wallet); err != nil {
			return err
		}
	} else {
		price, err := strconv.ParseFloat(walletConfig.GasPrices, 64)
		if err != nil {
			log.Error().Err(err).Msg("Invalid gas prices format")
			return err
		} else {
			log.Debug().Float64("gasPrice", price).Msg("Setting gas prices manually")
			lib.SetGasPrice(price)
		}
	}

	// WaitGroup for essential routines
	var wg sync.WaitGroup
	essentialDone := make(chan struct{}) // Channel for essential routines to signal when they are done

	// Run worker process per topic
	alreadyStartedWorkerForTopic := make(map[emissionstypes.TopicId]bool)
workerLoop:
	for _, worker := range suite.UserConfig.Worker {
		if _, ok := alreadyStartedWorkerForTopic[worker.TopicId]; ok {
			log.Warn().Uint64("topicId", worker.TopicId).Msg("Worker already started for topicId")
			continue
		}
		alreadyStartedWorkerForTopic[worker.TopicId] = true

		select {
		case <-suite.essentialCtx.Done():
			log.Info().Msg("Context cancelled, not starting more workers")
			break workerLoop // Exit loop
		default:
			wg.Add(1)
			go func(worker lib.WorkerConfig) {
				defer wg.Done()
				select {
				case <-suite.essentialCtx.Done():
					log.Info().Uint64("topicId", worker.TopicId).Msg("Worker process received shutdown signal")
					return
				default:
					suite.runWorkerProcess(suite.essentialCtx, worker)
				}
				log.Info().Uint64("topicId", worker.TopicId).Msg("Worker process finished")
			}(worker)
		}

		if lib.DoneOrWait(suite.essentialCtx, walletConfig.LaunchRoutineDelay) {
			log.Error().Msg("Worker process finished")
			suite.Metrics.IncrementMetricsCounter(metrics.WorkerProcessFinishedCount, wallet.Address, worker.TopicId)
		}
	}

	// Run reputer process per topic
	alreadyStartedReputerForTopic := make(map[emissionstypes.TopicId]bool)
reputerLoop:
	for _, reputer := range suite.UserConfig.Reputer {
		if _, ok := alreadyStartedReputerForTopic[reputer.TopicId]; ok {
			log.Warn().Uint64("topicId", reputer.TopicId).Msg("Reputer already started for topicId")
			continue
		}
		alreadyStartedReputerForTopic[reputer.TopicId] = true

		select {
		case <-suite.essentialCtx.Done():
			log.Info().Msg("Context cancelled, not starting more reputers")
			break reputerLoop // Exit loop
		default:
			wg.Add(1)
			go func(reputer lib.ReputerConfig) {
				defer wg.Done()
				select {
				case <-suite.essentialCtx.Done():
					log.Info().Uint64("topicId", reputer.TopicId).Msg("Reputer process received shutdown signal")
					return
				default:
					suite.runReputerProcess(suite.essentialCtx, reputer)
				}
				log.Info().Uint64("topicId", reputer.TopicId).Msg("Reputer process finished")
			}(reputer)
		}

		if lib.DoneOrWait(suite.essentialCtx, walletConfig.LaunchRoutineDelay) {
			log.Error().Msg("Reputer process finished")
			suite.Metrics.IncrementMetricsCounter(metrics.ReputerProcessFinishedCount, wallet.Address, reputer.TopicId)
		}
	}

	// Wait for all essential routines to finish
	go func() {
		wg.Wait()
		log.Info().Msg("All essential routines finished")
		close(essentialDone)
	}()

	<-essentialDone // Block until all essential routines are done
	log.Info().Msg("Essential routines channel unblocked")
	return nil
}

// Attempts to build and commit a worker payload for a given nonce
// Returns the nonce height acted upon (the received one or the new one if any)
func (suite *UseCaseSuite) processWorkerPayload(ctx context.Context, worker lib.WorkerConfig, latestNonceHeightActedUpon int64, timeoutHeight uint64) (int64, error) {
	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		return 0, errorsmod.Wrapf(err, "Error getting wallet config")
	}
	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		return 0, errorsmod.Wrapf(err, "Error getting wallet")
	}
	// Get latest nonce with RPC timeout
	latestOpenWorkerNonce, err := WithTimeoutResult(ctx, time.Duration(walletConfig.TimeoutRPCSecondsQuery)*time.Second,
		func(ctx context.Context) (*emissionstypes.Nonce, error) {
			node, err := suite.ConnectionManager.GetCurrentQueryNode()
			if err != nil {
				return nil, fmt.Errorf("failed to get current query node: %w", err)
			}
			return node.GetLatestOpenWorkerNonceByTopicId(ctx, worker.TopicId)
		})

	if err != nil {
		log.Warn().Err(err).Uint64("topicId", worker.TopicId).Msg("Error getting latest open worker nonce on topic - node availability issue?")
		return latestNonceHeightActedUpon, err
	}

	if latestOpenWorkerNonce.BlockHeight > latestNonceHeightActedUpon {
		// Check whitelist with RPC timeout
		isWhitelisted, err := WithTimeoutResult(ctx, time.Duration(walletConfig.TimeoutRPCSecondsQuery)*time.Second,
			func(ctx context.Context) (bool, error) {
				node, err := suite.ConnectionManager.GetCurrentQueryNode()
				if err != nil {
					return false, fmt.Errorf("failed to get current query node: %w", err)
				}
				return node.CanSubmitWorker(ctx, worker.TopicId, wallet.Address)
			})

		if err != nil {
			log.Error().Err(err).Uint64("topicId", worker.TopicId).Msg("Failed to check if worker is whitelisted")
			return latestNonceHeightActedUpon, err
		}
		if !isWhitelisted {
			log.Error().Uint64("topicId", worker.TopicId).Msg("Worker is not whitelisted in topic, not submitting payload")
			return latestOpenWorkerNonce.BlockHeight, nil
		}

		// Build and commit payload with transaction timeout
		err = WithTimeout(ctx, time.Duration(walletConfig.TimeoutRPCSecondsTx)*time.Second,
			func(ctx context.Context) error {
				return suite.BuildCommitWorkerPayload(ctx, worker, latestOpenWorkerNonce, timeoutHeight)
			})

		if err != nil {
			return latestNonceHeightActedUpon, errorsmod.Wrapf(err, "error building and committing worker payload for topic")
		}

		log.Debug().Uint64("topicId", worker.TopicId).
			Str("actorType", "worker").
			Msg("Successfully finished processing payload")
		return latestOpenWorkerNonce.BlockHeight, nil
	} else {
		log.Debug().Uint64("topicId", worker.TopicId).
			Int64("LastOpenNonceBlockHeight", latestOpenWorkerNonce.BlockHeight).
			Int64("latestNonceHeightActedUpon", latestNonceHeightActedUpon).Msg("No new worker nonce found")
		return latestNonceHeightActedUpon, nil
	}
}

func (suite *UseCaseSuite) processReputerPayload(ctx context.Context, reputer lib.ReputerConfig, latestNonceHeightActedUpon int64, timeoutHeight uint64) (int64, error) {
	log := log.With().Uint64("topicId", reputer.TopicId).Str("actorType", "reputer").Logger()
	log.Info().Msg("Processing reputer payload")
	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		return 0, errorsmod.Wrapf(err, "Error getting wallet config")
	}
	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		return 0, errorsmod.Wrapf(err, "Error getting wallet")
	}
	// Get nonce with RPC timeout

	nonce, err := lib.RunWithNodeRetry(
		ctx,
		suite.ConnectionManager,
		func(node *lib.NodeConfig) (*emissionstypes.Nonce, error) {
			return WithTimeoutResult(ctx,
				time.Duration(walletConfig.TimeoutRPCSecondsQuery)*time.Second,
				func(ctx context.Context) (*emissionstypes.Nonce, error) {
					return node.GetOldestReputerNonceByTopicId(ctx, reputer.TopicId)
				})
		},
		"get oldest reputer nonce",
		lib.GRPC_MODE,
	)
	if err != nil {
		log.Warn().Err(err).Msg("Error getting latest open reputer nonce on topic - node availability issue?")
		return latestNonceHeightActedUpon, err
	}

	if nonce.BlockHeight > latestNonceHeightActedUpon {
		// Check if reputer can submit
		isWhitelisted, err := lib.RunWithNodeRetry(
			ctx,
			suite.ConnectionManager,
			func(node *lib.NodeConfig) (bool, error) {
				return WithTimeoutResult(ctx,
					time.Duration(walletConfig.TimeoutRPCSecondsQuery)*time.Second,
					func(ctx context.Context) (bool, error) {
						return node.CanSubmitReputer(ctx, reputer.TopicId, wallet.Address)
					})
			},
			"check reputer whitelist",
			lib.GRPC_MODE,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to check if reputer is whitelisted")
			return latestNonceHeightActedUpon, err
		}
		if !isWhitelisted {
			log.Error().Msg("Reputer is not whitelisted in topic, not submitting payload")
			return nonce.BlockHeight, nil
		}

		// Build and commit payload with transaction timeout
		err = WithTimeout(ctx, time.Duration(walletConfig.TimeoutRPCSecondsTx)*time.Second,
			func(ctx context.Context) error {
				return suite.BuildCommitReputerPayload(ctx, reputer, nonce.BlockHeight, timeoutHeight)
			})

		if err != nil {
			return latestNonceHeightActedUpon, errorsmod.Wrapf(err, "error building and committing reputer payload for topic")
		}

		log.Debug().Msg("Successfully finished processing payload")
		return nonce.BlockHeight, nil
	} else {
		log.Debug().
			Int64("LastOpenNonceBlockHeight", nonce.BlockHeight).
			Int64("latestNonceHeightActedUpon", latestNonceHeightActedUpon).Msg("No new reputer nonce found")
		return latestNonceHeightActedUpon, nil
	}
}

// Calculate the time distance based on the distance until the next epoch
func calculateTimeDistanceInSeconds(distanceUntilNextEpoch int64, blockDurationAvg, correctionFactor float64) (int64, error) {
	if distanceUntilNextEpoch < 0 || correctionFactor < 0 {
		return 0, errors.New("distanceUntilNextEpoch and correctionFactor must be positive")
	}
	correctedTimeDistance := float64(distanceUntilNextEpoch) * blockDurationAvg * correctionFactor
	return int64(math.Round(correctedTimeDistance)), nil
}

// Generate jitter between 0 and submissionJitter
func generateRandomJitter(submissionJitter uint64) int64 {
	if submissionJitter == 0 {
		return 0
	}
	source := rand.NewSource(uint64(time.Now().UnixNano())) // nolint: gosec
	rng := rand.New(source)

	maxSafeValue := uint64(math.MaxInt64)
	if submissionJitter > maxSafeValue {
		submissionJitter = maxSafeValue
	}
	return int64(rng.Uint64() % submissionJitter) //nolint:gosec // using a safe max value
}

// Runs the worker process for a given worker config
func (suite *UseCaseSuite) runWorkerProcess(ctx context.Context, worker lib.WorkerConfig) {
	// Create a logger with the topicId
	log := log.With().Uint64("topicId", worker.TopicId).Str("actorType", "worker").Logger()
	log.Info().Msg("Running worker process for topic")

	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get wallet config")
		return
	}
	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get wallet")
		return
	}
	// Handle registration
	registered, err := lib.RunWithNodeRetry(
		ctx,
		suite.ConnectionManager,
		func(node *lib.NodeConfig) (bool, error) {
			return WithTimeoutResult(ctx, time.Duration(walletConfig.TimeoutRPCSecondsRegistration)*time.Second,
				func(ctx context.Context) (bool, error) {
					return suite.RegisterWorkerIdempotently(ctx, worker)
				})
		},
		"RegisterWorkerIdempotently",
		lib.RPC_MODE,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to register for topic, exiting")
		return
	}

	if !registered {
		log.Error().Msg("Failed to register worker for topic, exiting")
		return
	}
	log.Debug().Msg("Worker registered")

	// Using the helper function
	topicInfo, err := queryTopicInfo(ctx, suite, worker)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get topic info for worker")
		return
	}

	getNonce := func(ctx context.Context, topicId emissionstypes.TopicId) (*emissionstypes.Nonce, error) {
		return lib.RunWithNodeRetry(
			ctx,
			suite.ConnectionManager,
			func(node *lib.NodeConfig) (*emissionstypes.Nonce, error) {
				return node.GetLatestOpenWorkerNonceByTopicId(ctx, topicId)
			},
			"get latest open worker nonce",
			lib.GRPC_MODE,
		)
	}
	params := ActorProcessParams[lib.WorkerConfig]{
		Config:                 worker,
		ProcessPayload:         suite.processWorkerPayload,
		GetNonce:               getNonce,
		NearWindowLength:       topicInfo.WorkerSubmissionWindow, // Use worker window to determine "nearness"
		SubmissionWindowLength: topicInfo.WorkerSubmissionWindow, // Use worker window for actual submission window
		ActorType:              "worker",
	}

	// Check if worker is isWhitelisted
	isWhitelisted, err := lib.RunWithNodeRetry(
		ctx,
		suite.ConnectionManager,
		func(node *lib.NodeConfig) (bool, error) {
			return WithTimeoutResult(ctx, time.Duration(walletConfig.TimeoutRPCSecondsQuery)*time.Second,
				func(ctx context.Context) (bool, error) {
					return node.CanSubmitWorker(ctx, worker.TopicId, wallet.Address)
				})
		},
		"check worker whitelist",
		lib.GRPC_MODE,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check if worker is whitelisted")
		return
	}
	if !isWhitelisted {
		log.Error().Msg("Worker is not whitelisted in topic, exiting worker process")
		return
	}

	// Run the actor process
	runActorProcess(ctx, suite, params)
}

// Runs the reputer process for a given reputer config
func (suite *UseCaseSuite) runReputerProcess(ctx context.Context, reputer lib.ReputerConfig) {
	// Create a logger with the topicId
	log := log.With().Uint64("topicId", reputer.TopicId).Str("actorType", "reputer").Logger()
	log.Debug().Msg("Running reputer process for topic")
	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get wallet config")
		return
	}
	wallet, err := suite.ConnectionManager.GetWallet()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get wallet")
		return
	}
	// Handle registration and staking
	registeredAndStaked, err := lib.RunWithNodeRetry(
		ctx,
		suite.ConnectionManager,
		func(node *lib.NodeConfig) (bool, error) {
			return WithTimeoutResult(ctx,
				time.Duration(walletConfig.TimeoutRPCSecondsRegistration)*time.Second,
				func(ctx context.Context) (bool, error) {
					return suite.RegisterAndStakeReputerIdempotently(ctx, reputer)
				})
		},
		"RegisterAndStakeReputerIdempotently",
		lib.RPC_MODE,
	)
	if err != nil {
		log.Error().Err(err).Msg("Error: Failed to register or sufficiently stake for topic")
		return
	}
	if !registeredAndStaked {
		log.Error().Msg("Could not register or sufficiently stake for topic")
		return
	}
	log.Debug().Msg("Reputer registered and staked")

	// Using the helper function
	topicInfo, err := queryTopicInfo(ctx, suite, reputer)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get topic info for reputer")
		return
	}

	getNonce := func(ctx context.Context, topicId emissionstypes.TopicId) (*emissionstypes.Nonce, error) {
		return lib.RunWithNodeRetry(
			ctx,
			suite.ConnectionManager,
			func(node *lib.NodeConfig) (*emissionstypes.Nonce, error) {
				return node.GetOldestReputerNonceByTopicId(ctx, topicId)
			},
			"get oldest reputer nonce",
			lib.GRPC_MODE,
		)
	}
	params := ActorProcessParams[lib.ReputerConfig]{
		Config:                 reputer,
		ProcessPayload:         suite.processReputerPayload,
		GetNonce:               getNonce,
		NearWindowLength:       topicInfo.WorkerSubmissionWindow, // Use worker window to determine "nearness"
		SubmissionWindowLength: topicInfo.EpochLength,            // Use epoch length for actual submission window
		ActorType:              "reputer",
	}

	// Check if reputer is isWhitelisted
	isWhitelisted, err := lib.RunWithNodeRetry(
		ctx,
		suite.ConnectionManager,
		func(node *lib.NodeConfig) (bool, error) {
			return WithTimeoutResult(ctx,
				time.Duration(walletConfig.TimeoutRPCSecondsQuery)*time.Second,
				func(ctx context.Context) (bool, error) {
					return node.CanSubmitReputer(ctx, reputer.TopicId, wallet.Address)
				})
		},
		"check reputer whitelist",
		lib.GRPC_MODE,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check if reputer is whitelisted")
		return
	}
	if !isWhitelisted {
		log.Error().Msg("Reputer is not whitelisted in topic, exiting reputer process")
		return
	}

	// Run the actor process
	runActorProcess(ctx, suite, params)
}

// Function that runs the actor process for a given topic and actor type
// This mechanism is used to handle the submission of payloads for both workers and reputers,
// using ActorProcessParams to handle the different configurations and functions needed for each actor type
func runActorProcess[T lib.TopicActor](ctx context.Context, suite *UseCaseSuite, params ActorProcessParams[T]) {
	// Create a logger with the topicId and actorType
	log := log.With().Uint64("topicId", params.Config.GetTopicId()).Str("actorType", params.ActorType).Logger()
	log.Debug().Msg("Running actor process for topic")

	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get wallet config")
		return
	}

	topicInfo, err := queryTopicInfo(ctx, suite, params.Config)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get topic info after retries")
		return
	}

	epochLength := topicInfo.EpochLength
	minBlocksToCheck := params.NearWindowLength * NUM_SUBMISSION_WINDOWS_FOR_SUBMISSION_NEARNESS
	latestNonceHeightSentTxFor := int64(0)
	var currentBlockHeight int64

	for {
		log.Trace().Msg("Start iteration, querying latest block")
		// Query the latest block
		currentBlockHeight, err = WithTimeoutResult(ctx, time.Duration(walletConfig.TimeoutRPCSecondsQuery)*time.Second,
			func(ctx context.Context) (lib.BlockHeight, error) {
				node, err := suite.ConnectionManager.GetCurrentQueryNode()
				if err != nil {
					return 0, fmt.Errorf("failed to get current query node: %w", err)
				}
				return node.GetBlockHeight(ctx, walletConfig)
			})

		if err != nil {
			log.Error().Err(err).Msg("Failed to get status")
			if lib.DoneOrWait(ctx, WAIT_TIME_STATUS_CHECKS) {
				return
			}
			continue
		}

		topicInfo, err := queryTopicInfo(ctx, suite, params.Config)
		if err != nil {
			log.Error().Err(err).Msg("Error getting topic info")
			return
		}
		log.Trace().
			Int64("currentBlockHeight", currentBlockHeight).
			Int64("EpochLastEnded", topicInfo.EpochLastEnded).
			Int64("EpochLength", epochLength).
			Msg("Info from topic")

		// Special case: new topic
		if topicInfo.EpochLastEnded == 0 {
			log.Debug().Msg("New topic, processing payload")
			// timeoutHeight is one epoch length away
			timeoutHeight := currentBlockHeight + epochLength

			latestNonceHeightSentTxFor, err = params.ProcessPayload(ctx, params.Config, latestNonceHeightSentTxFor, uint64(timeoutHeight)) // nolint: gosec
			if err != nil {
				log.Error().Err(err).Msg("Error processing payload - could not complete transaction")
			}
			// Wait for an epochLength with a correction factor, it will self-adjust from there
			waitingTimeInSeconds, err := calculateTimeDistanceInSeconds(
				epochLength,
				walletConfig.BlockDurationEstimated,
				NEW_TOPIC_CORRECTION_FACTOR,
			)
			if err != nil {
				log.Error().Err(err).Int64("waitingTimeInSeconds", waitingTimeInSeconds).Msg("Error calculating time distance to next epoch after sending tx - wait epochLength")
				return
			}
			if lib.DoneOrWait(ctx, waitingTimeInSeconds) {
				log.Info().Err(ctx.Err()).Msg("Context done")
				return
			}
			continue
		}

		epochLastEnded := topicInfo.EpochLastEnded
		epochEnd := epochLastEnded + epochLength
		timeoutHeight := epochLastEnded + params.SubmissionWindowLength
		log.Trace().
			Int64("epochLastEnded", epochLastEnded).
			Int64("epochEnd", epochEnd).
			Int64("timeoutHeight", timeoutHeight).
			Msg("Epoch info")

		var waitingTimeInSeconds int64

		// Check if block is within the submission window
		if currentBlockHeight-epochLastEnded <= params.SubmissionWindowLength {
			// Within the submission window, attempt to process payload
			latestNonceHeightSentTxFor, err = params.ProcessPayload(ctx, params.Config, latestNonceHeightSentTxFor, uint64(timeoutHeight)) // nolint: gosec
			if err != nil {
				log.Error().Err(err).Msg("Error processing payload - could not complete transaction")
			}

			distanceUntilNextEpoch := epochEnd - currentBlockHeight
			if distanceUntilNextEpoch < 0 {
				log.Warn().
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("submissionWindowLength", params.SubmissionWindowLength).
					Msg("Distance until next epoch is less than 0, setting to submissionWindowLength")
				distanceUntilNextEpoch = params.SubmissionWindowLength
			}

			waitingTimeInSeconds, err = calculateTimeDistanceInSeconds(
				distanceUntilNextEpoch,
				walletConfig.BlockDurationEstimated,
				walletConfig.WindowCorrectionFactor,
			)
			if err != nil {
				log.Error().Err(err).Msg("Error calculating time distance to next epoch after sending tx")
				return
			}

			log.Info().
				Int64("currentBlockHeight", currentBlockHeight).
				Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
				Int64("waitingTimeInSeconds", waitingTimeInSeconds).
				Msg("Waiting until the submission window opens after sending")
		} else if currentBlockHeight > epochEnd {
			// Inconsistent topic data, wait until the next epoch
			waitingTimeInSeconds, err = calculateTimeDistanceInSeconds(
				epochLength,
				walletConfig.BlockDurationEstimated,
				NEARNESS_CORRECTION_FACTOR,
			)
			if err != nil {
				log.Error().Err(err).Msg("Error calculating time distance to next epoch after sending tx")
				return
			}
			log.Warn().
				Int64("waitingTimeInSeconds", waitingTimeInSeconds).
				Int64("currentBlockHeight", currentBlockHeight).
				Int64("epochEnd", epochEnd).
				Msg("Current block height is greater than next epoch length, is topic inactive? Waiting seconds...")
		} else {
			distanceUntilNextEpoch := epochEnd - currentBlockHeight
			if distanceUntilNextEpoch <= minBlocksToCheck {
				// Close distance, check more closely until the submission window opens
				// Introduce a random jitter to avoid thundering herd problem
				jitter := generateRandomJitter(walletConfig.SubmissionJitter)
				closeBlockDistance := distanceUntilNextEpoch + jitter
				waitingTimeInSeconds, err = calculateTimeDistanceInSeconds(
					closeBlockDistance,
					walletConfig.BlockDurationEstimated,
					NEARNESS_CORRECTION_FACTOR,
				)
				if err != nil {
					log.Error().Err(err).Msg("Error calculating close distance to epochLength")
					return
				}
				log.Info().
					Int64("SubmissionWindowLength", params.SubmissionWindowLength).
					Int64("jitter", jitter).
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("closeBlockDistance", closeBlockDistance).
					Int64("waitingTimeInSeconds", waitingTimeInSeconds).
					Msg("Close to the window, waiting until next submission window")
			} else {
				// Far distance, bigger waits until the submission window opens
				waitingTimeInSeconds, err = calculateTimeDistanceInSeconds(
					distanceUntilNextEpoch,
					walletConfig.BlockDurationEstimated,
					walletConfig.WindowCorrectionFactor,
				)
				if err != nil {
					log.Error().Err(err).Msg("Error calculating far distance to epochLength")
					return
				}
				log.Info().
					Int64("currentBlockHeight", currentBlockHeight).
					Int64("distanceUntilNextEpoch", distanceUntilNextEpoch).
					Int64("waitingTimeInSeconds", waitingTimeInSeconds).
					Msg("Waiting until the submission window opens - far distance")
			}
		}
		if lib.DoneOrWait(ctx, waitingTimeInSeconds) {
			log.Info().Err(ctx.Err()).Msg("Context done")
			return
		}
	}
}

// Queries the topic info for a given actor type and wallet params from suite
// Wrapper over NodeConfig.GetTopicInfo() with generic config type
func queryTopicInfo[T lib.TopicActor](
	ctx context.Context,
	suite *UseCaseSuite,
	config T,
) (*emissionstypes.Topic, error) {
	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		return nil, errorsmod.Wrapf(err, "Error getting wallet config")
	}
	topicInfo, err := WithTimeoutResult(ctx,
		time.Duration(walletConfig.TimeoutRPCSecondsQuery)*time.Second,
		func(ctx context.Context) (*emissionstypes.Topic, error) {
			node, err := suite.ConnectionManager.GetCurrentQueryNode()
			if err != nil {
				return nil, fmt.Errorf("failed to get current query node: %w", err)
			}
			return node.GetTopicInfo(ctx, config.GetTopicId())
		})
	if err != nil {
		return nil, errorsmod.Wrapf(err, "failed to get topic info")
	}
	return topicInfo, nil
}

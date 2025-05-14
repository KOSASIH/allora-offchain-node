package usecase

import (
	lib "allora_offchain_node/lib"
	metrics "allora_offchain_node/metrics"
	"context"
	"strconv"

	errorsmod "cosmossdk.io/errors"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/rs/zerolog/log"
)

// True if the actor is ultimately, definitively registered for the specified topic, else False
// Idempotent in registration
func (suite *UseCaseSuite) RegisterWorkerIdempotently(ctx context.Context, config lib.WorkerConfig) (bool, error) {
	log := log.With().Uint64("topicId", config.TopicId).Str("actorType", "worker").Logger()
	log.Info().Msg("Registering worker")

	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		return false, errorsmod.Wrapf(err, "Error getting wallet config")
	}
	connectionManager := suite.ConnectionManager
	queryNode, err := connectionManager.GetCurrentQueryNode()
	if err != nil {
		log.Error().Err(err).Msg("Could not get current query node")
		return false, err
	}
	wallet, err := connectionManager.GetWallet()
	if err != nil {
		log.Error().Err(err).Msg("Could not get wallet")
		return false, err
	}

	isRegistered, err := queryNode.IsWorkerRegistered(ctx, config.TopicId)
	if err != nil {
		log.Error().Err(err).Str("rpc", queryNode.ServerAddress).Msg("Could not check if the node is already registered for topic as worker, skipping")
		return false, err
	}
	if isRegistered {
		log.Info().Msg("Already registered for topic")
		return true, nil
	} else {
		log.Info().Msg("Node not yet registered. Attempting registration...")
	}

	queryNode, err = connectionManager.SwitchToNextQueryNode()
	if err != nil {
		log.Error().Err(err).Msg("Could not switch to next query node")
		return false, err
	}
	moduleParams, err := queryNode.Chain.EmissionsQueryClient.GetParams(ctx, &emissionstypes.GetParamsRequest{})
	if err != nil {
		log.Error().Err(err).Msg("Could not get chain params")
		return false, err
	}

	// Switch to spread the load
	queryNode, err = connectionManager.SwitchToNextQueryNode()
	if err != nil {
		log.Error().Err(err).Msg("Could not switch to next query node")
		return false, err
	}

	balance, err := queryNode.GetBalance(ctx, wallet.Address, wallet.GetDefaultBondDenom())
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node has enough balance to register, skipping")
		return false, err
	}
	if !balance.GTE(moduleParams.Params.RegistrationFee) {
		log.Error().Str("balance", balance.String()).Msg("Node does not have enough balance to register, skipping.")
		suite.Metrics.IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, wallet.Address, strconv.Itoa(lib.ErrCodeNotEnoughBalance))
		return false, lib.ErrNotEnoughBalance
	}

	msg := &emissionstypes.RegisterRequest{
		Sender:    wallet.Address,
		TopicId:   config.TopicId,
		Owner:     wallet.Address,
		IsReputer: false,
	}
	res, err := connectionManager.SendDataWithRetry(ctx, msg, "Register node", 0)
	if err != nil {
		if lib.IsErrorSwitchingNode(err) {
			log.Warn().Err(err).Msg("Error on worker registration process, switching to next node")
			return false, err
		}

		txHash := ""
		if res != nil {
			txHash = res.Hash.String()
		}
		log.Error().Err(err).Str("txHash", txHash).Msg("Could not register the node with the Allora blockchain")
		suite.Metrics.IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, wallet.Address, strconv.Itoa(lib.ErrCodeNotRegistered))
		return false, err
	}

	// Give time for the tx to be included in a block
	delay := int64(walletConfig.BlockDurationEstimated * float64(walletConfig.RegistrationWaitingBlocks))
	log.Debug().Int64("delay", delay).Msg("Waiting to check registration status to be included in a block...")
	if lib.DoneOrWait(ctx, delay) {
		log.Error().Err(ctx.Err()).Str("rpc", queryNode.ServerAddress).Msg("Waiting to check registration status failed")
		return false, ctx.Err()
	}
	isRegistered, err = queryNode.IsWorkerRegistered(ctx, config.TopicId)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node is already registered for topic, skipping")
		return false, err
	}

	return isRegistered, nil
}

// True if the actor is ultimately, definitively registered for the specified topic with at least config.MinStake placed on topic, else False
// Actor may be either a worker or a reputer
// Idempotent in registration and stake addition
func (suite *UseCaseSuite) RegisterAndStakeReputerIdempotently(ctx context.Context, config lib.ReputerConfig) (bool, error) {
	log := log.With().Uint64("topicId", config.TopicId).Str("actorType", "reputer").Logger()
	log.Info().Msg("Registering reputer")

	walletConfig, err := suite.ConnectionManager.GetWalletConfig()
	if err != nil {
		return false, errorsmod.Wrapf(err, "Error getting wallet config")
	}
	connectionManager := suite.ConnectionManager
	queryNode, err := connectionManager.GetCurrentQueryNode()
	if err != nil {
		log.Error().Err(err).Msg("Could not get current query node")
		return false, err
	}
	wallet, err := connectionManager.GetWallet()
	if err != nil {
		log.Error().Err(err).Msg("Could not get wallet")
		return false, err
	}

	isRegistered, err := queryNode.IsReputerRegistered(ctx, config.TopicId)
	if err != nil {
		log.Error().Err(err).Str("rpc", queryNode.ServerAddress).Msg("Could not check if the node is already registered for topic as reputer, skipping")
		return false, err
	}

	if isRegistered {
		log.Info().Msg("Already registered")
	} else {
		log.Info().Msg("Node not yet registered. Attempting registration...")

		balance, err := queryNode.GetBalance(ctx, wallet.Address, wallet.GetDefaultBondDenom())
		if err != nil {
			log.Error().Err(err).Msg("Could not check if the node has enough balance to register, skipping")
			return false, err
		}
		moduleParams, err := queryNode.Chain.EmissionsQueryClient.GetParams(ctx, &emissionstypes.GetParamsRequest{})
		if err != nil {
			log.Error().Err(err).Str("rpc", queryNode.ServerAddress).Msg("Could not get chain params for reputer")
			return false, err
		}
		if !balance.GTE(moduleParams.Params.RegistrationFee) {
			log.Error().Msg("Node does not have enough balance to register, skipping.")
			suite.Metrics.IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, wallet.Address, strconv.Itoa(lib.ErrCodeNotEnoughBalance))
			return false, lib.ErrNotEnoughBalance
		}

		msgRegister := &emissionstypes.RegisterRequest{
			Sender:    wallet.Address,
			TopicId:   config.TopicId,
			Owner:     wallet.Address,
			IsReputer: true,
		}
		res, err := connectionManager.SendDataWithRetry(ctx, msgRegister, "Register node", 0)
		if err != nil {
			if lib.IsErrorSwitchingNode(err) {
				log.Warn().Err(err).Msg("Error on reputer registration process, switching to next node")
				return false, err
			}
			txHash := ""
			if res != nil {
				txHash = res.Hash.String()
			}
			log.Error().Err(err).Str("txHash", txHash).Msg("Could not register the node with the Allora blockchain")
			suite.Metrics.IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, wallet.Address, strconv.Itoa(lib.ErrCodeCannotAddStake))
			return false, err
		}

		// Give time for the tx to be included in a block
		delay := int64(walletConfig.BlockDurationEstimated * float64(walletConfig.RegistrationWaitingBlocks))
		log.Debug().Int64("delay", delay).Msg("Waiting to check registration status to be included in a block...")
		if lib.DoneOrWait(ctx, delay) {
			log.Error().Err(ctx.Err()).Str("rpc", queryNode.ServerAddress).Msg("Waiting to check registration status failed")
			return false, ctx.Err()
		}
		isRegistered, err = queryNode.IsReputerRegistered(ctx, config.TopicId)
		if err != nil {
			log.Error().Err(err).Msg("Could not check if the node is already registered for topic, skipping")
			return false, err
		}
		if !isRegistered {
			log.Error().Msg("Node not registered after all retries")
			suite.Metrics.IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, wallet.Address, strconv.Itoa(lib.ErrCodeNotRegistered))
			return false, lib.ErrNotRegistered
		}
	}

	stake, err := queryNode.GetReputerStakeInTopic(ctx, config.TopicId, wallet.Address)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node has enough balance to stake, skipping")
		return false, err
	}

	minStake := config.MinStake.Number
	if minStake.IsNil() {
		log.Info().Msg("No minimum stake configured in reputer, skipping adding stake.")
		return true, nil
	}
	if minStake.IsZero() {
		log.Info().Msg("No minimum stake requested, skipping adding stake.")
		return true, nil
	}
	if minStake.LTE(stake) {
		log.Info().Interface("stake", stake).Interface("minStake", minStake).Msg("Stake above minimum requested stake, skipping adding stake.")
		return true, nil
	} else {
		log.Info().Interface("stake", stake).Interface("minStake", minStake).Interface("stakeToAdd", minStake.Sub(stake)).Msg("Stake below minimum requested stake, adding stake.")
	}

	msgAddStake := &emissionstypes.AddStakeRequest{
		Sender:  wallet.Address,
		Amount:  minStake.Sub(stake),
		TopicId: config.TopicId,
	}
	res, err := connectionManager.SendDataWithRetry(ctx, msgAddStake, "Add stake", 0)
	if err != nil {
		// Necessary to switch to next node if we get a 429 error handling control to caller
		if lib.IsErrorSwitchingNode(err) {
			log.Warn().Err(err).Msg("Error adding stake, switching to next node")
			return false, err
		}

		txHash := ""
		if res != nil {
			txHash = res.Hash.String()
		}
		log.Error().Err(err).Str("txHash", txHash).Msg("Could not stake the node with the Allora blockchain in specified topic")
		suite.Metrics.IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, wallet.Address, strconv.Itoa(lib.ErrCodeCannotAddStake))
		return false, err
	}

	// Give time for the tx to be included in a block
	delay := int64(walletConfig.BlockDurationEstimated * float64(walletConfig.RegistrationWaitingBlocks))
	log.Debug().Int64("delay", delay).Msg("Waiting to check stake status to be included in a block...")
	if lib.DoneOrWait(ctx, delay) {
		log.Error().Err(ctx.Err()).Str("rpc", queryNode.ServerAddress).Msg("Waiting to check stake status failed")
		return false, ctx.Err()
	}
	stake, err = queryNode.GetReputerStakeInTopic(ctx, config.TopicId, wallet.Address)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node has enough balance to stake, skipping")
		return false, err
	}
	if stake.LT(minStake) {
		log.Error().Interface("stake", stake).Interface("minStake", minStake).Msg("Stake below minimum requested stake, skipping.")
		suite.Metrics.IncrementMetricsCounterWithLabels(metrics.ActorTxErrorCount, wallet.Address, strconv.Itoa(lib.ErrCodeStakeBelowMin))
		return false, lib.ErrStakeBelowMin
	}

	return true, nil
}

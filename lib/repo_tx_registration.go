package lib

import (
	"context"

	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/rs/zerolog/log"
)

// True if the actor is ultimately, definitively registered for the specified topic, else False
// Idempotent in registration
func (node *NodeConfig) RegisterWorkerIdempotently(ctx context.Context, config WorkerConfig) (bool, error) {
	log := log.With().Uint64("topicId", config.TopicId).Str("actorType", "worker").Logger()
	log.Info().Msg("Registering worker")

	isRegistered, err := node.IsWorkerRegistered(ctx, config.TopicId)
	if err != nil {
		log.Error().Err(err).Str("rpc", node.RPC).Msg("Could not check if the node is already registered for topic as worker, skipping")
		return false, err
	}
	if isRegistered {
		log.Info().Msg("Already registered for topic")
		return true, nil
	} else {
		log.Info().Msg("Node not yet registered. Attempting registration...")
	}

	moduleParams, err := node.Chain.EmissionsQueryClient.GetParams(ctx, &emissionstypes.GetParamsRequest{})
	if err != nil {
		log.Error().Err(err).Msg("Could not get chain params")
		return false, err
	}

	balance, err := node.GetBalance(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node has enough balance to register, skipping")
		return false, err
	}
	if !balance.GTE(moduleParams.Params.RegistrationFee) {
		log.Error().Str("balance", balance.String()).Msg("Node does not have enough balance to register, skipping.")
		return false, ErrNotEnoughBalance
	}

	msg := &emissionstypes.RegisterRequest{
		Sender:    node.Chain.Address,
		TopicId:   config.TopicId,
		Owner:     node.Chain.Address,
		IsReputer: false,
	}
	res, err := node.SendDataWithRetry(ctx, msg, "Register node", 0)
	if err != nil {
		if IsErrorSwitchingNode(err) {
			log.Warn().Err(err).Str("rpc", node.RPC).Msg("Error on worker registration process, switching to next node")
			return false, err
		}

		txHash := ""
		if res != nil {
			txHash = res.TxHash
		}
		log.Error().Err(err).Str("txHash", txHash).Msg("Could not register the node with the Allora blockchain")
		return false, err
	}

	// Give time for the tx to be included in a block
	log.Debug().Int64("delay", node.Wallet.RetryDelay).Msg("Waiting to check registration status to be included in a block...")
	if DoneOrWait(ctx, node.Wallet.RetryDelay) {
		log.Error().Err(ctx.Err()).Str("rpc", node.RPC).Msg("Waiting to check registration status failed")
		return false, ctx.Err()
	}
	isRegistered, err = node.IsWorkerRegistered(ctx, config.TopicId)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node is already registered for topic, skipping")
		return false, err
	}

	return isRegistered, nil
}

// True if the actor is ultimately, definitively registered for the specified topic with at least config.MinStake placed on topic, else False
// Actor may be either a worker or a reputer
// Idempotent in registration and stake addition
func (node *NodeConfig) RegisterAndStakeReputerIdempotently(ctx context.Context, config ReputerConfig) (bool, error) {
	log := log.With().Uint64("topicId", config.TopicId).Str("actorType", "reputer").Logger()
	log.Info().Msg("Registering reputer")

	isRegistered, err := node.IsReputerRegistered(ctx, config.TopicId)
	if err != nil {
		log.Error().Err(err).Str("rpc", node.RPC).Msg("Could not check if the node is already registered for topic as reputer, skipping")
		return false, err
	}

	if isRegistered {
		log.Info().Msg("Already registered")
	} else {
		log.Info().Msg("Node not yet registered. Attempting registration...")

		balance, err := node.GetBalance(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Could not check if the node has enough balance to register, skipping")
			return false, err
		}
		moduleParams, err := node.Chain.EmissionsQueryClient.GetParams(ctx, &emissionstypes.GetParamsRequest{})
		if err != nil {
			log.Error().Err(err).Str("rpc", node.RPC).Msg("Could not get chain params for reputer")
			return false, err
		}
		if !balance.GTE(moduleParams.Params.RegistrationFee) {
			log.Error().Msg("Node does not have enough balance to register, skipping.")
			return false, ErrNotEnoughBalance
		}

		msgRegister := &emissionstypes.RegisterRequest{
			Sender:    node.Chain.Address,
			TopicId:   config.TopicId,
			Owner:     node.Chain.Address,
			IsReputer: true,
		}
		res, err := node.SendDataWithRetry(ctx, msgRegister, "Register node", 0)
		if err != nil {
			if IsErrorSwitchingNode(err) {
				log.Warn().Err(err).Str("rpc", node.RPC).Msg("Error on reputer registration process, switching to next node")
				return false, err
			}
			txHash := ""
			if res != nil {
				txHash = res.TxHash
			}
			log.Error().Err(err).Str("txHash", txHash).Msg("Could not register the node with the Allora blockchain")
			return false, err
		}

		// Give time for the tx to be included in a block
		log.Debug().Int64("delay", node.Wallet.RetryDelay).Msg("Waiting to check registration status to be included in a block...")
		if DoneOrWait(ctx, node.Wallet.RetryDelay) {
			log.Error().Err(ctx.Err()).Str("rpc", node.RPC).Msg("Waiting to check registration status failed")
			return false, ctx.Err()
		}
		isRegistered, err = node.IsReputerRegistered(ctx, config.TopicId)
		if err != nil {
			log.Error().Err(err).Msg("Could not check if the node is already registered for topic, skipping")
			return false, err
		}
		if !isRegistered {
			log.Error().Msg("Node not registered after all retries")
			return false, ErrNotRegistered
		}
	}

	stake, err := node.GetReputerStakeInTopic(ctx, config.TopicId, node.Chain.Address)
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
		Sender:  node.Wallet.Address,
		Amount:  minStake.Sub(stake),
		TopicId: config.TopicId,
	}
	res, err := node.SendDataWithRetry(ctx, msgAddStake, "Add stake", 0)
	if err != nil {
		// Necessary to switch to next node if we get a 429 error handling control to caller
		if IsErrorSwitchingNode(err) {
			log.Warn().Err(err).Str("rpc", node.RPC).Msg("Error adding stake, switching to next node")
			return false, err
		}

		txHash := ""
		if res != nil {
			txHash = res.TxHash
		}
		log.Error().Err(err).Str("txHash", txHash).Msg("Could not stake the node with the Allora blockchain in specified topic")
		return false, err
	}

	// Give time for the tx to be included in a block
	log.Debug().Int64("delay", node.Wallet.RetryDelay).Msg("Waiting to check stake status to be included in a block...")
	if DoneOrWait(ctx, node.Wallet.RetryDelay) {
		log.Error().Err(ctx.Err()).Str("rpc", node.RPC).Msg("Waiting to check stake status failed")
		return false, ctx.Err()
	}
	stake, err = node.GetReputerStakeInTopic(ctx, config.TopicId, node.Chain.Address)
	if err != nil {
		log.Error().Err(err).Msg("Could not check if the node has enough balance to stake, skipping")
		return false, err
	}
	if stake.LT(minStake) {
		log.Error().Interface("stake", stake).Interface("minStake", minStake).Msg("Stake below minimum requested stake, skipping.")
		return false, ErrStakeBelowMin
	}

	return true, nil
}

package lib

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	cosmossdk_io_math "cosmossdk.io/math"
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
)

// True if the actor is ultimately, definitively registered for the specified topic, else False
// Idempotent in registration
func (node *NodeConfig) RegisterWorkerIdempotently(ctx context.Context, config WorkerConfig) (bool, error) {
	isRegistered, err := node.IsWorkerRegistered(ctx, config.TopicId)
	if err != nil {
		log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not check if the node is already registered for topic as worker, skipping")
		return false, err
	}
	if isRegistered {
		log.Info().Str("rpc", node.Wallet.NodeRpc).Uint64("topicId", config.TopicId).Msg("Worker node already registered for topic")
		return true, nil
	} else {
		log.Info().Str("rpc", node.Wallet.NodeRpc).Uint64("topicId", config.TopicId).Msg("Worker node not yet registered for topic. Attempting registration...")
	}

	moduleParams, err := node.Chain.EmissionsQueryClient.GetParams(ctx, &emissionstypes.GetParamsRequest{})
	if err != nil {
		log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not get chain params for worker ")
		return false, err
	}

	balance, err := node.GetBalance(ctx)
	if err != nil {
		log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not check if the worker node has enough balance to register, skipping")
		return false, err
	}
	if !balance.GTE(moduleParams.Params.RegistrationFee) {
		log.Error().Str("rpc", node.Wallet.NodeRpc).Str("balance", balance.String()).Msg("Worker node does not have enough balance to register, skipping.")
		return false, fmt.Errorf("worker node does not have enough balance to register, skipping")
	}

	msg := &emissionstypes.RegisterRequest{
		Sender:    node.Chain.Address,
		TopicId:   config.TopicId,
		Owner:     node.Chain.Address,
		IsReputer: false,
	}
	res, err := node.SendDataWithRetry(ctx, msg, "Register worker node", 0)
	if err != nil {

		txHash := ""
		if res != nil {
			txHash = res.TxHash
		}
		log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Uint64("topic", config.TopicId).Str("txHash", txHash).Msg("Could not register the worker node with the Allora blockchain")
		return false, err
	}

	// Give time for the tx to be included in a block
	log.Debug().Int64("delay", node.Wallet.RetryDelay).Msg("Waiting to check registration status to be included in a block...")
	if DoneOrWait(ctx, node.Wallet.RetryDelay) {
		log.Error().Err(ctx.Err()).Str("rpc", node.Wallet.NodeRpc).Msg("Waiting to check registration status failed")
		return false, ctx.Err()
	}
	isRegistered, err = node.IsWorkerRegistered(ctx, config.TopicId)
	if err != nil {
		log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not check if the node is already registered for topic as worker, skipping")
		return false, err
	}

	return isRegistered, nil
}

// True if the actor is ultimately, definitively registered for the specified topic with at least config.MinStake placed on topic, else False
// Actor may be either a worker or a reputer
// Idempotent in registration and stake addition
func (node *NodeConfig) RegisterAndStakeReputerIdempotently(ctx context.Context, config ReputerConfig) (bool, error) {
	isRegistered, err := node.IsReputerRegistered(ctx, config.TopicId)
	if err != nil {
		log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not check if the node is already registered for topic as reputer, skipping")
		return false, err
	}

	if isRegistered {
		log.Info().Uint64("topicId", config.TopicId).Msg("Reputer node already registered")
	} else {
		log.Info().Uint64("topicId", config.TopicId).Msg("Reputer node not yet registered. Attempting registration...")

		balance, err := node.GetBalance(ctx)
		if err != nil {
			log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not check if the Reputer node has enough balance to register, skipping")
			return false, err
		}
		moduleParams, err := node.Chain.EmissionsQueryClient.GetParams(ctx, &emissionstypes.GetParamsRequest{})
		if err != nil {
			log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not get chain params for reputer")
			return false, err
		}
		if !balance.GTE(moduleParams.Params.RegistrationFee) {
			log.Error().Str("rpc", node.Wallet.NodeRpc).Msg("Reputer node does not have enough balance to register, skipping.")
			return false, fmt.Errorf("reputer node does not have enough balance to register, skipping")
		}

		msgRegister := &emissionstypes.RegisterRequest{
			Sender:    node.Chain.Address,
			TopicId:   config.TopicId,
			Owner:     node.Chain.Address,
			IsReputer: true,
		}
		res, err := node.SendDataWithRetry(ctx, msgRegister, "Register reputer node", 0)
		if err != nil {

			txHash := ""
			if res != nil {
				txHash = res.TxHash
			}
			log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Uint64("topic", config.TopicId).Str("txHash", txHash).Msg("Could not register the reputer node with the Allora blockchain")
			return false, err
		}

		// Give time for the tx to be included in a block
		log.Debug().Int64("delay", node.Wallet.RetryDelay).Msg("Waiting to check registration status to be included in a block...")
		if DoneOrWait(ctx, node.Wallet.RetryDelay) {
			log.Error().Err(ctx.Err()).Str("rpc", node.Wallet.NodeRpc).Msg("Waiting to check registration status failed")
			return false, ctx.Err()
		}
		isRegistered, err = node.IsReputerRegistered(ctx, config.TopicId)
		if err != nil {
			log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not check if the node is already registered for topic as reputer, skipping")
			return false, err
		}
		if !isRegistered {
			log.Error().Str("rpc", node.Wallet.NodeRpc).Uint64("topicId", config.TopicId).Msg("Reputer node not registered after all retries")
			return false, fmt.Errorf("reputer node not registered after all retries")
		}
	}

	stake, err := node.GetReputerStakeInTopic(ctx, config.TopicId, node.Chain.Address)
	if err != nil {
		log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not check if the reputer node has enough balance to stake, skipping")
		return false, err
	}

	minStake := cosmossdk_io_math.NewInt(config.MinStake)
	if minStake.LTE(stake) {
		log.Info().Str("rpc", node.Wallet.NodeRpc).Msg("Reputer stake above minimum requested stake, skipping adding stake.")
		return true, nil
	} else {
		log.Info().Str("rpc", node.Wallet.NodeRpc).Interface("stake", stake).Interface("minStake", minStake).Interface("stakeToAdd", minStake.Sub(stake)).Msg("Reputer stake below minimum requested stake, adding stake.")
	}

	msgAddStake := &emissionstypes.AddStakeRequest{
		Sender:  node.Wallet.Address,
		Amount:  minStake.Sub(stake),
		TopicId: config.TopicId,
	}
	res, err := node.SendDataWithRetry(ctx, msgAddStake, "Add reputer stake", 0)
	if err != nil {
		txHash := ""
		if res != nil {
			txHash = res.TxHash
		}
		log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Uint64("topic", config.TopicId).Str("txHash", txHash).Msg("Could not stake the reputer node with the Allora blockchain in specified topic")
		return false, err
	}

	// Give time for the tx to be included in a block
	log.Debug().Int64("delay", node.Wallet.RetryDelay).Msg("Waiting to check stake status to be included in a block...")
	if DoneOrWait(ctx, node.Wallet.RetryDelay) {
		log.Error().Err(ctx.Err()).Str("rpc", node.Wallet.NodeRpc).Msg("Waiting to check stake status failed")
		return false, ctx.Err()
	}
	stake, err = node.GetReputerStakeInTopic(ctx, config.TopicId, node.Chain.Address)
	if err != nil {
		log.Error().Err(err).Str("rpc", node.Wallet.NodeRpc).Msg("Could not check if the reputer node has enough balance to stake, skipping")
		return false, err
	}
	if stake.LT(minStake) {
		log.Error().Str("rpc", node.Wallet.NodeRpc).Interface("stake", stake).Interface("minStake", minStake).Msg("Reputer stake below minimum requested stake, skipping.")
		return false, fmt.Errorf("reputer stake below minimum requested stake, skipping")
	}

	return true, nil
}

package main

import (
	"allora_offchain_node/lib"
	usecase "allora_offchain_node/usecase"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

func ConvertEntrypointsToInstances(userConfig lib.UserConfig) error {
	/// Initialize adapters using the factory function
	for i, worker := range userConfig.Worker {
		if worker.InferenceEntrypointName != "" {
			adapter, err := NewAlloraAdapter(worker.InferenceEntrypointName)
			if err != nil {
				fmt.Println("Error creating inference adapter:", err)
				return err
			}
			userConfig.Worker[i].InferenceEntrypoint = adapter
		}

		if worker.ForecastEntrypointName != "" {
			adapter, err := NewAlloraAdapter(worker.ForecastEntrypointName)
			if err != nil {
				fmt.Println("Error creating forecast adapter:", err)
				return err
			}
			userConfig.Worker[i].ForecastEntrypoint = adapter
		}
	}

	for i, reputer := range userConfig.Reputer {
		if reputer.GroundTruthEntrypointName != "" {
			adapter, err := NewAlloraAdapter(reputer.GroundTruthEntrypointName)
			if err != nil {
				fmt.Println("Error creating reputer adapter:", err)
				return err
			}
			userConfig.Reputer[i].GroundTruthEntrypoint = adapter
		}
	}

	for i, reputer := range userConfig.Reputer {
		if reputer.LossFunctionEntrypointName != "" {
			adapter, err := NewAlloraAdapter(reputer.LossFunctionEntrypointName)
			if err != nil {
				fmt.Println("Error creating reputer adapter:", err)
				return err
			}
			userConfig.Reputer[i].LossFunctionEntrypoint = adapter
		}
	}
	return nil
}

func main() {
	initLogger()
	if dotErr := godotenv.Load(); dotErr != nil {
		log.Info().Msg("Unable to load .env file")
	}

	log.Info().Msg("Starting allora offchain node...")

	metrics := lib.NewMetrics(lib.CounterData)
	metrics.RegisterMetricsCounters()
	metrics.StartMetricsServer(":2112")

	finalUserConfig := lib.UserConfig{} // nolint: exhaustruct
	alloraJsonConfig := os.Getenv(lib.ALLORA_OFFCHAIN_NODE_CONFIG_JSON)
	if alloraJsonConfig != "" {
		log.Info().Msg("Config using JSON env var")
		// completely reset UserConfig
		err := json.Unmarshal([]byte(alloraJsonConfig), &finalUserConfig)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to parse JSON config file from Config")
			return
		}
	} else if os.Getenv(lib.ALLORA_OFFCHAIN_NODE_CONFIG_FILE_PATH) != "" {
		log.Info().Msg("Config using JSON config file")
		// parse file defined in CONFIG_FILE_PATH into UserConfig
		file, err := os.Open(os.Getenv(lib.ALLORA_OFFCHAIN_NODE_CONFIG_FILE_PATH))
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to open JSON config file")
			return
		}
		defer file.Close()
		decoder := json.NewDecoder(file)
		// completely reset UserConfig
		err = decoder.Decode(&finalUserConfig)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to parse JSON config file")
			return
		}
	} else {
		log.Fatal().Msg("Could not find config file. Please create a config.json file and pass as environment variable.")
		return
	}

	// Convert entrypoints to instances of adapters
	err := ConvertEntrypointsToInstances(finalUserConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to convert Entrypoints to instances of adapters - wrong entrypoint name?")
		return
	}
	spawner, err := usecase.NewUseCaseSuite(finalUserConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize use case, exiting")
		return
	}

	spawner.Metrics = *metrics

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCtx, sigCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer sigCancel()

	go func() {
		spawner.Spawn(sigCtx)
		cancel()
	}()

	<-sigCtx.Done()

	log.Info().Msg("Stopping...")

	<-ctx.Done()
}

# allora-offchain-node

Allora off-chain nodes publish inferences, forecasts, and losses informed by a configurable ground truth and applying a configurable loss function to the Allora chain.

## How to run with docker
1. Clone the repository
2. Make sure to remove any .env file so it doesn't clash with the automated environment variables
3. Copy config.example.json and populate with your variables. You can either populate with your existing wallet or leave it empty for it to be autocreated

```shell
cp config.example.json config.json
```
4. Run command below to load your config.json file to environment

```shell
chmod +x init.config
./init.config
```

from the root directory. This will:
   - Load your config.json file into the environment. Depending on whether you provided your wallet details or not it will also do the following:
      - Automatically create allora keys for you. You will have to request for some tokens from faucet to be able to register your worker and stake your reputer. You can find your address in ./data/env_file
      - Automatically export the needed variables from the account created to be used by the offchain node and bundles it with the your provided config.json and then pass them to the node as environemnt variable

5. Run `docker compose up --build`. This will:
   - Run the both the offchain node and the source services, communicating through endpoints attached to the internal dns

Please note that the environment variable will be created as bumdle of your config.json and allora account secrets, please make sure to remove every secrets before commiting to remote git repository


## How to run without docker

1. Clone the repository
2. Install Go 1.22.5
3. Install the dependencies:

```shell
go mod download
```

4. Copy environment variables:

```shell
cp .env.example .env
```

5. Fill in the environment variables in `.env` with your own values
6. If you're a worker...
   1. Configure your inference and/or forecast models in `adapters/` and/or another source linked to by adapters in `adapters/` directory.
7. If you're a reputer...
   1. Configure your repute and/or loss models in `adapters/` and/or another source linked to by adapters in `adapters/` directory.
8. Map each topic to the appropriate adapter in `config.json`.
9. Run the following commands:

```shell
chmod +x start.local
./start.local
```

## Prometheus Metrics
Some metrics has been provided for in the node. You can access them with port `:2112/metrics`. Here are the following list of existing metrics: 
- `allora_worker_inference_request_count`: The total number of times worker requests inference from source
- `allora_worker_forecast_request_count`: The total number of times worker requests forecast from source
- `allora_reputer_truth_request_count`: The total number of times reputer requests truth from source
- `allora_worker_data_build_count`: The total number of times worker built data successfully
- `allora_reputer_data_build_count`: The total number of times reputer built data successfully
- `allora_worker_chain_submission_count`: The total number of worker commits to the chain
- `allora_reputer_chain_submission_count`: The total number of reputer commits to the chain

> Please note that we will keep updating the list as more metrics are being added

## Cycle Example

Visualization of timeline and submission windows for an actor, in this case, a worker. 
Reputers have submission windows too, but they are fixed to be 1 full topic's epoch length.

Example configuration:
* Topic epoch length is 100 blocks
* Topic submission window is 10 blocks
* Near zone is 2 submission windows (20 blocks)

```
                                   Epoch N                                        Epoch N+1
|---------|----------------------------------|-----------------------------------|--------→
Block:    1000                              1100                                1200
          ↑                                  ↑                                   ↑
          Epoch Start                        Epoch End                          Next Epoch End
          (& Submission                      & Next Epoch Start
          Window Start)                      (& Next Submission
                                            Window Start)

Detailed View of Zones (assuming epoch length = 100 blocks):

Block 1000  Block 1010                Block 1080    Block 1100
    |-----------|--------------------------|--------------|
    |←                Far Zone            →|←  Near Zone →|
    |←   SW    →|
(SW = Submission Window)

Zone Breakdown (example numbers):
• Epoch Length: 100 blocks
• Submission Window: 100 blocks (coincides with epoch)
• Near Zone: Last 20 blocks (NUM_SUBMISSION_WINDOWS_FOR_SUBMISSION_NEARNESS * WorkerSubmissionWindow)

-------------------------------
- Full cycle transition points
  - Block 1000: Epoch N starts & Submission Window opens
  - Block 1010: Submission Window closes. Waiting for next window, typically from far zone.
  - Block 1080: Enters Near Zone (more frequent checks)
  - Block 1100: Epoch N ends & Epoch N+1 starts

```

### Notes

- Submissions
  - Submissions are accepted within the submission window
  - Submission window opens at epoch start
- Waiting Zone Behavior
  - The behaviour of the node when waiting for the submission window depends on its nearness to the submission window to reduce likelihood of missing a window.
  - Far Zone: Longer intervals between checks, optimized for efficiency
    - This is controlled by `blockDurationEstimated` and `windowCorrectionFactor`
  - Near Zone: More frequent checks with randomization for fair participation
  - Submissions are separated - they must happen within the submission window

### Random offset

The node introduces a random offset to the submission time to avoid the thundering herd problem alleviating mempool congestion.

## How to configure

There are several ways to configure the node. In order of preference, you can do any of these: 
* Set the `ALLORA_OFFCHAIN_NODE_CONFIG_JSON` env var with a configuration as a JSON string.
* Set the `ALLORA_OFFCHAIN_NODE_CONFIG_FILE_PATH` env var pointing to a file, which contains configuration as JSON. An example if provided in `config.example.json`.

Each option completely overwrites the other options.

This is the entrypoint for the application that simply builds and runs the Go program.

It spins off a distinct processes per role worker, reputer per topic configered in `config.json`.

## Logging env vars

* LOG_LEVEL: Set the logging level. Valid values are `debug`, `info`, `warn`, `error`, `fatal`, `panic`. Defaults to `info`.
* LOG_TIME_FORMAT: Sets the format of the timestamp in the log. Valid values are `unix`, `unixms`, `unixmicro`, `iso8601`. Defaults to `iso8601`.

## Wallet configuration

The wallet configuration is done in `config.json` under the `wallet` field.

### Gas and Fees

Please see detailed information in [gas_and_fees.md](gas_and_fees.md).

### Timeout configuration

The node will use the following timeouts:
* `timeoutRPCSecondsQuery`: Timeout for RPC queries in seconds, including retries.
* `timeoutRPCSecondsTx`: Timeout for RPC sending data in seconds, including retries.
* `timeoutRPCSecondsRegistration`: Timeout for whole RPC registration process in seconds, including retries.

### Error handling

Error handling is done differently for different types of errors.
Note: when an account sequence mismatch is detected, the node will attempt to set the expected sequence number and retry the transaction before broadcasting.
Note: the node will check if the actor is whitelisted on worker setup and before submitting a payload.

#### Retries 
- `accounSequenceRetryDelay`: For the "account sequence mismatch" error. 
- `retryDelay`: For all other errors that need retry delays.


### Smart Window Detection

The node will automatically detect the submission window length for each topic on each actor type.
This can be configured by the following settings in the config.json:
* `blockDurationEstimated`: Estimated network block time in seconds. Minimum is 1.
* `windowCorrectionFactor`: Correction factor to fine-tune the submission window length. Higher values optimize the number of calls for window checking. Minimum is 0.5.

## Configuration examples

A complete example is provided in `config.example.json`. 
These below are excerpts of the configuration (with some parts omitted for brevity) for different setups:

### 1 workers as inferer 

```json
{
   "worker": [
      {
        "topicId": 1,
        "inferenceEntrypointName": "apiAdapter",
        "parameters": {
          "InferenceEndpoint": "http://source:8000/inference/{Token}",
          "Token": "ETH"
        }
      }
   ]
}
```

###  1 worker as forecaster
```json
{
   "worker": [
      {
        "topicId": 1,
        "forecastEntrypointName": "apiAdapter",
        "parameters": {
          "ForecastEndpoint": "http://source:8000/forecasts/{TopicId}/{BlockHeight}"
        }
      }
   ]
}

```

###  1 worker as inferer and forecaster

```json
{
   "worker": [
      {
        "topicId": 1,
        "inferenceEntrypointName": "apiAdapter",
        "forecastEntrypointName": "apiAdapter",
        "parameters": {
          "InferenceEndpoint": "http://source:8000/inference/{Token}",
          "ForecastEndpoint": "http://source:8000/forecasts/{TopicId}/{BlockHeight}",
          "Token": "ETH"
        }
      }
   ]
}
```

### 1 reputer

```json
{
"reputer": [
      {
        "topicId": 1,
        "groundTruthEntrypointName": "apiAdapter",
        "lossFunctionEntrypointName": "apiAdapter",
        "minStake": 100000,
        "groundTruthParameters": {
          "GroundTruthEndpoint": "http://localhost:8888/gt/{Token}/{BlockHeight}",
          "Token": "ETHUSD"
        },
        "lossFunctionParameters": {
          "LossFunctionService": "http://localhost:5000",
          "LossMethodOptions": {
            "loss_method": "sqe"
          }
        }
      }
    ]
}
```

### 1 worker as inferer and forecaster, and 1 reputer

```json
{
"worker": [
      {
        "topicId": 1,
        "inferenceEntrypointName": "apiAdapter",
        "forecastEntrypointName": "apiAdapter",
        "parameters": {
          "InferenceEndpoint": "http://source:8000/inference/{Token}",
          "ForecastEndpoint": "http://source:8000/forecasts/{TopicId}/{BlockHeight}",
          "Token": "ETH"
        }
      }
    ],
"reputer": [
      {
        "topicId": 1,
        "groundTruthEntrypointName": "apiAdapter",
        "lossFunctionEntrypointName": "apiAdapter",
        "minStake": 100000,
        "groundTruthParameters": {
          "GroundTruthEndpoint": "http://localhost:8888/gt/{Token}/{BlockHeight}",
          "Token": "ETHUSD"
        },
        "lossFunctionParameters": {
          "LossFunctionService": "http://localhost:5000",
          "LossMethodOptions": {
            "loss_method": "sqe"
          }
        }
      }
    ]
}
```

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.

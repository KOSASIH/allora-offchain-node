package metrics

// Default labels used by most counters
var DefaultLabels = []string{"address", "topic"}
var EndpointLabels = []string{"endpoint"}
var ErrorCodeLabels = []string{"address", "error_code"}

// metrics
const (
	InferenceRequestCount          string = "allora_worker_inference_request_count"
	ForecastRequestCount           string = "allora_worker_forecast_request_count"
	TruthRequestCount              string = "allora_reputer_truth_request_count"
	WorkerChainSubmissionCount     string = "allora_worker_chain_submission_count"
	ReputerChainSubmissionCount    string = "allora_reputer_chain_submission_count"
	WorkerProcessFinishedCount     string = "allora_worker_process_finished_count"
	ReputerProcessFinishedCount    string = "allora_reputer_process_finished_count"
	ApplicationStartedCount        string = "allora_application_started_count"
	GRPCConnectionLostCount        string = "allora_grpc_connection_lost_count"
	GRPCReconnectionCount          string = "allora_grpc_reconnection_count"
	GRPCConnectionPermanentFailure string = "allora_grpc_connection_permanent_failure"
	ActorTxErrorCount              string = "allora_actor_tx_error_count"
)

// A struct that holds the name and help text for a prometheus counter
var CounterData = []MetricsCounter{
	{InferenceRequestCount, "The total number of times worker requests inference from source", DefaultLabels},
	{ForecastRequestCount, "The total number of times worker requests forecast from source", DefaultLabels},
	{TruthRequestCount, "The total number of times reputer requests truth from source", DefaultLabels},
	{WorkerChainSubmissionCount, "The total number of worker commits to the chain", DefaultLabels},
	{ReputerChainSubmissionCount, "The total number of reputer commits to the chain", DefaultLabels},
	{WorkerProcessFinishedCount, "The total number of worker processes finished", DefaultLabels},
	{ReputerProcessFinishedCount, "The total number of reputer processes finished", DefaultLabels},
	{ApplicationStartedCount, "The total number of application started", DefaultLabels},
	{GRPCConnectionLostCount, "The total number of times the GRPC connection is lost", EndpointLabels},
	{GRPCReconnectionCount, "The total number of times the GRPC connection is successfully reconnected", EndpointLabels},
	{GRPCConnectionPermanentFailure, "The total number of times the GRPC connection is lost", EndpointLabels},
	{ActorTxErrorCount, "The total number of errors by error code", ErrorCodeLabels},
}

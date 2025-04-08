package grpcclient

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"time"

	"allora_offchain_node/metrics"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type backoffConfig struct {
	initial    time.Duration
	max        time.Duration
	maxRetries int
	jitterFrac float64 // fraction of the backoff to use for jitter
}

func newBackoffConfig() backoffConfig {
	return backoffConfig{
		initial:    1 * time.Second,
		max:        30 * time.Second,
		maxRetries: 5,
		jitterFrac: 0.2, // 20% jitter
	}
}

func (bc *backoffConfig) nextBackoff(current time.Duration) time.Duration {
	// Calculate base backoff with exponential increase
	next := time.Duration(math.Min(float64(current*2), float64(bc.max)))

	// Apply jitter: randomly subtract up to jitterFrac of the duration
	jitterRange := float64(next) * bc.jitterFrac

	// Generate cryptographically secure random number between 0 and jitterRange
	maxJitter := big.NewInt(int64(jitterRange))
	randomBig, err := rand.Int(rand.Reader, maxJitter)
	if err != nil {
		// If we fail to generate random jitter, just return the next backoff without jitter
		log.Warn().Err(err).Msg("Failed to generate jitter, continuing without it")
		return next
	}

	jitter := time.Duration(randomBig.Int64())
	return next - jitter
}

func monitorGRPCConnection(ctx context.Context, grpcConnection *grpc.ClientConn, grpcEndpoint string) {
	if grpcConnection == nil {
		log.Error().Msg("nil gRPC connection provided to monitor")
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	bc := newBackoffConfig()
	retryCount := 0
	backoff := bc.initial

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Shutting down gRPC monitoring goroutine")
			return
		case <-ticker.C:
			state := grpcConnection.GetState()

			// Only attempt reconnection if we're in a failed state
			if state != connectivity.TransientFailure && state != connectivity.Shutdown {
				continue
			}
			metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.GRPCConnectionLostCount, grpcEndpoint)
			log.Warn().Msg("gRPC Connection lost, attempting to reconnect...")
			if err := attemptReconnection(ctx, grpcConnection, &retryCount, &backoff, bc, grpcEndpoint); err != nil {
				return // Monitor shutdown due to max retries or context cancellation
			}
		}
	}
}

func attemptReconnection(
	ctx context.Context,
	conn *grpc.ClientConn,
	retryCount *int,
	backoff *time.Duration,
	bc backoffConfig,
	endpoint string,
) error {
	// Force reconnection attempt
	conn.ResetConnectBackoff()
	conn.Connect()

	if conn.GetState() == connectivity.Ready {
		log.Info().Msg("gRPC connection restored")
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.GRPCReconnectionCount, endpoint)
		*retryCount = 0
		*backoff = bc.initial
		return nil
	}

	*retryCount++
	log.Warn().
		Int("retry", *retryCount).
		Dur("backoff", *backoff).
		Msg("Reconnection attempt failed")

	if *retryCount >= bc.maxRetries {
		metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.GRPCConnectionPermanentFailure, endpoint)
		log.Error().Msg("Max reconnection attempts reached, triggering shutdown")
		return fmt.Errorf("max reconnection retries exceeded")
	}

	// Wait for backoff duration or context cancellation
	timer := time.NewTimer(*backoff)
	defer timer.Stop()

	select {
	case <-timer.C:
		*backoff = bc.nextBackoff(*backoff)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context cancelled during backoff: %w", ctx.Err())
	}
}

// InitializeGRPCClient initializes a gRPC client for the given endpoint with proper connection monitoring
func InitializeGRPCClient(ctx context.Context, grpcEndpoint string, insecureFlag bool) (*grpc.ClientConn, error) {
	dialOptions := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(8*1024*1024),
			grpc.MaxCallSendMsgSize(8*1024*1024),
			grpc.ForceCodec(&customCodec{}),
		),
	}

	// Configure transport security
	if insecureFlag {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds := credentials.NewClientTLSFromCert(nil, "")
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	}

	log.Debug().
		Interface("dialOptions", dialOptions).
		Str("target", grpcEndpoint).
		Msg("Initializing gRPC client with options")

	log.Info().Msg("Creating new gRPC client")
	grpcConnection, err := grpc.NewClient(
		grpcEndpoint,
		dialOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", grpcEndpoint, err)
	}

	// Start connection monitoring in a separate goroutine
	go monitorGRPCConnection(ctx, grpcConnection, grpcEndpoint)

	return grpcConnection, nil
}

// An interceptor that logs the gRPC request, for debugging purposes
// func loggerHeaderInterceptor() grpc.UnaryClientInterceptor {
// 	return func(
// 		ctx context.Context,
// 		method string,
// 		req, reply interface{},
// 		cc *grpc.ClientConn,
// 		invoker grpc.UnaryInvoker,
// 		opts ...grpc.CallOption,
// 	) error {
// 		log.Info().Str("method", method).Msg("Intercepting gRPC request")
// 		return invoker(ctx, method, req, reply, cc, opts...)
// 	}
// }

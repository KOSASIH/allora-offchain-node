package grpcclient

import (
	"context"
	"fmt"
	"time"

	"allora_offchain_node/metrics"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// monitorGRPCConnection monitors the gRPC connection state and attempts to reconnect when needed.
func monitorGRPCConnection(ctx context.Context, grpcConnnection *grpc.ClientConn, grpcEndpoint string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done(): // Graceful shutdown
			log.Info().Msg("Shutting down gRPC monitoring goroutine.")
			return

		case <-ticker.C:
			state := grpcConnnection.GetState()
			if state == connectivity.TransientFailure || state == connectivity.Shutdown {
				log.Warn().Msg("gRPC Connection lost, attempting to reconnect...")

				// Exponential backoff for reconnection attempts
				backoff := time.Second
				maxBackoff := 30 * time.Second

				for {
					select {
					case <-ctx.Done():
						log.Info().Msg("Stopping reconnection attempts due to shutdown.")
						return

					default:
						// Wait for a state change
						if grpcConnnection.WaitForStateChange(ctx, state) {
							log.Info().Msg("gRPC connection state changed, resuming normal operation.")
							metrics.GetMetrics().IncrementMetricsCounterWithLabels(metrics.GRPCConnectionLostCount, grpcEndpoint)
							break
						}

						// Increase backoff exponentially, up to maxBackoff
						time.Sleep(backoff)
						if backoff < maxBackoff {
							backoff *= 2
						}
					}
				}
			}
		}
	}
}

// Initializes a gRPC client for the given endpoint
func InitializeGRPCClient(ctx context.Context, grpcEndpoint string, insecureFlag bool) (grpcConnection *grpc.ClientConn, err error) {
	var dialOptions []grpc.DialOption

	kaOpts := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             10 * time.Second,
		PermitWithoutStream: true,
	}

	customCodec := &customCodec{}
	dialOptions = append(dialOptions, grpc.WithKeepaliveParams(kaOpts))
	dialOptions = append(dialOptions,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(8*1024*1024),
			grpc.MaxCallSendMsgSize(8*1024*1024),
			grpc.ForceCodec(customCodec),
		),
	)

	if insecureFlag {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds := credentials.NewClientTLSFromCert(nil, "")
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	}

	// Add interceptor for logging if needed
	// dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(loggerHeaderInterceptor()))
	log.Debug().Interface("dialOptions", dialOptions).Str("target", grpcEndpoint).Msg("Dial options")
	log.Info().Msg("Creating new gRPC client")
	grpcConnection, err = grpc.NewClient(
		grpcEndpoint,
		dialOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", grpcEndpoint, err)
	}

	// spin up goroutine for monitoring and reconnect purposes - TODO test and configure
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

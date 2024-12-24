package usecase

import (
	"context"
	"fmt"
	"time"
)

// WithTimeoutResult applies a timeout to a function and handles cleanup of the context.
func WithTimeoutResult[T any](ctx context.Context, timeout time.Duration, fn func(ctx context.Context) (T, error)) (T, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel() // Ensure resources are cleaned up right after the operation

	result, err := fn(timeoutCtx)

	// Handle context cancellation or timeout explicitly
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return *new(T), fmt.Errorf("operation timed out: %w", timeoutCtx.Err())
	} else if timeoutCtx.Err() == context.Canceled {
		return *new(T), fmt.Errorf("operation canceled: %w", timeoutCtx.Err())
	}

	return result, err
}

// WithTimeout applies a timeout to a function that only returns an error using an empty struct as a placeholder
func WithTimeout(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	_, err := WithTimeoutResult(ctx, timeout, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

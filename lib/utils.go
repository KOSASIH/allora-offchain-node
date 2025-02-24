package lib

import (
	"context"
	"time"
)

// DoneOrWait returns true if ctx.Done() arrived first
func DoneOrWait(ctx context.Context, seconds int64) bool {
	// Validate input
	if seconds < 0 {
		return false
	}

	// Create timer once instead of using time.After
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop() // Ensure timer is cleaned up

	select {
	case <-ctx.Done():
		return true
	case <-timer.C:
		return false
	}
}

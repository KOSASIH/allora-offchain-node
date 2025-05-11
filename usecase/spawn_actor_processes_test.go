package usecase

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateTimeDistanceInSeconds(t *testing.T) {
	tests := []struct {
		name                   string
		distanceUntilNextEpoch int64
		correctionFactor       float64
		blockAvgSeconds        float64
		expectedTimeDistance   int64
		expectedError          bool
	}{
		{
			name:                   "Basic calculation",
			distanceUntilNextEpoch: 100,
			correctionFactor:       1.0,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   460, // 100 * 4.6 * 1.0
			expectedError:          false,
		},
		{
			name:                   "With correction factor",
			distanceUntilNextEpoch: 100,
			correctionFactor:       0.75,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   345, // 100 * 4.6 * 0.75
			expectedError:          false,
		},
		{
			name:                   "Zero distance",
			distanceUntilNextEpoch: 0,
			correctionFactor:       1.0,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   0,
			expectedError:          false,
		},
		{
			name:                   "Large distance",
			distanceUntilNextEpoch: 1000000,
			correctionFactor:       1.0,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   4600000, // 1000000 * 4.6 * 1.0
			expectedError:          false,
		},
		{
			name:                   "Small correction factor",
			distanceUntilNextEpoch: 100,
			correctionFactor:       0.1,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   46, // 100 * 4.6 * 0.1
			expectedError:          false,
		},
		{
			name:                   "Negative distance",
			distanceUntilNextEpoch: -100,
			correctionFactor:       1.0,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   0,
			expectedError:          true,
		},
		{
			name:                   "Negative correction factor",
			distanceUntilNextEpoch: 100,
			correctionFactor:       -0.5,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   0,
			expectedError:          true,
		},
		{
			name:                   "Both negative",
			distanceUntilNextEpoch: -100,
			correctionFactor:       -0.5,
			blockAvgSeconds:        4.6,
			expectedTimeDistance:   0,
			expectedError:          true,
		},
		// tests with different blockAvgSeconds
		{
			name:                   "Basic calculation with 6s blocks",
			distanceUntilNextEpoch: 100,
			correctionFactor:       1.0,
			blockAvgSeconds:        6.0,
			expectedTimeDistance:   600, // 100 * 6.0 * 1.0
			expectedError:          false,
		},
		{
			name:                   "With correction factor and 3s blocks",
			distanceUntilNextEpoch: 100,
			correctionFactor:       0.75,
			blockAvgSeconds:        3.0,
			expectedTimeDistance:   225, // 100 * 3.0 * 0.75
			expectedError:          false,
		},
		{
			name:                   "Large distance with 5.5s blocks",
			distanceUntilNextEpoch: 1000000,
			correctionFactor:       1.0,
			blockAvgSeconds:        5.5,
			expectedTimeDistance:   5500000, // 1000000 * 5.5 * 1.0
			expectedError:          false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := calculateTimeDistanceInSeconds(test.distanceUntilNextEpoch, test.blockAvgSeconds, test.correctionFactor)
			if test.expectedError {
				require.Error(t, err, "Expected an error for negative input")
				assert.Equal(t, int64(0), result, "Expected 0 result when error occurs")
			} else {
				require.NoError(t, err, "Did not expect an error")
				assert.Equal(t, test.expectedTimeDistance, result, "Calculated time distance should match expected value")
			}
		})
	}
}

func TestGenerateRandomJitter(t *testing.T) {
	tests := []struct {
		name             string
		submissionJitter uint64
		expectedMin      int64
		expectedMax      int64
		iterations       int
	}{
		{
			name:             "Zero jitter",
			submissionJitter: 0,
			expectedMin:      0,
			expectedMax:      0,
			iterations:       1000,
		},
		{
			name:             "Small jitter",
			submissionJitter: 10,
			expectedMin:      0,
			expectedMax:      9, // Since it's modulo, max will be submissionJitter - 1
			iterations:       10000,
		},
		{
			name:             "Medium jitter",
			submissionJitter: 100,
			expectedMin:      0,
			expectedMax:      99,
			iterations:       10000,
		},
		{
			name:             "Large jitter",
			submissionJitter: 1000,
			expectedMin:      0,
			expectedMax:      999,
			iterations:       10000,
		},
		{
			name:             "Power of two jitter",
			submissionJitter: 256,
			expectedMin:      0,
			expectedMax:      255,
			iterations:       10000,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for range make([]struct{}, test.iterations) {
				result := generateRandomJitter(test.submissionJitter)
				fmt.Println(result)

				// Check bounds
				assert.GreaterOrEqual(t, result, test.expectedMin,
					"Result should be greater than or equal to the minimum value")
				assert.LessOrEqual(t, result, test.expectedMax,
					"Result should be less than or equal to the maximum value")
			}
		})
	}
}

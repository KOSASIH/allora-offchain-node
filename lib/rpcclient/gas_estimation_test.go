package rpcclient

import (
	"math"
	"math/big"
	"testing"

	types "allora_offchain_node/lib/types"

	cosmossdk_io_math "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
)

func TestEstimateGas(t *testing.T) {
	tests := []struct {
		name        string
		txSize      int
		config      types.GasEstimationConfig
		expectGas   uint64
		expectError bool
	}{
		{
			name:   "Normal case",
			txSize: 1000,
			config: types.GasEstimationConfig{
				BaseGas:       100000,
				GasPerByte:    10,
				MinGasPrice:   0,
				OverrideFees:  0,
				SimulateTx:    false,
				GasAdjustment: 1.0,
				OverrideGas:   0,
			},
			expectGas:   110000, // 100000 + (1000 * 10)
			expectError: false,
		},
		{
			name:   "Zero tx size",
			txSize: 0,
			config: types.GasEstimationConfig{
				BaseGas:       100000,
				GasPerByte:    10,
				MinGasPrice:   0,
				OverrideFees:  0,
				SimulateTx:    false,
				GasAdjustment: 1.0,
				OverrideGas:   0,
			},
			expectGas:   100000, // just base gas
			expectError: false,
		},
		{
			name:   "Negative tx size",
			txSize: -1,
			config: types.GasEstimationConfig{
				BaseGas:       100000,
				GasPerByte:    10,
				MinGasPrice:   0,
				OverrideFees:  0,
				SimulateTx:    false,
				GasAdjustment: 1.0,
				OverrideGas:   0,
			},
			expectGas:   0,
			expectError: true,
		},
		{
			name:   "Zero base gas",
			txSize: 1000,
			config: types.GasEstimationConfig{
				BaseGas:       0,
				GasPerByte:    10,
				MinGasPrice:   0,
				OverrideFees:  0,
				SimulateTx:    false,
				GasAdjustment: 1.0,
				OverrideGas:   0,
			},
			expectGas:   10000, // just size gas
			expectError: false,
		},
		{
			name:   "Zero gas per byte",
			txSize: 1000,
			config: types.GasEstimationConfig{
				BaseGas:       100000,
				GasPerByte:    0,
				MinGasPrice:   0,
				OverrideFees:  0,
				SimulateTx:    false,
				GasAdjustment: 1.0,
				OverrideGas:   0,
			},
			expectGas:   100000, // just base gas
			expectError: false,
		},
		{
			name:   "Large tx size",
			txSize: math.MaxInt32,
			config: types.GasEstimationConfig{
				BaseGas:       100000,
				GasPerByte:    10,
				MinGasPrice:   0,
				OverrideFees:  0,
				SimulateTx:    false,
				GasAdjustment: 1.0,
				OverrideGas:   0,
			},
			expectGas:   21474936470, // should overflow
			expectError: false,
		},
		{
			name:   "Large base gas",
			txSize: 1000,
			config: types.GasEstimationConfig{
				BaseGas:       math.MaxUint64,
				GasPerByte:    10,
				MinGasPrice:   0,
				OverrideFees:  0,
				SimulateTx:    false,
				GasAdjustment: 1.0,
				OverrideGas:   0,
			},
			expectGas:   0, // should overflow
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gas, err := EstimateGas(tt.txSize, tt.config)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectGas, gas)
			}
		})
	}
}

func TestCalculateFees(t *testing.T) {
	tests := []struct {
		name        string
		gas         uint64
		minGasPrice float64
		expectFee   cosmossdk_io_math.Int
		expectError bool
		epsilon     cosmossdk_io_math.Int
	}{
		{
			name:        "Normal case",
			gas:         100000,
			minGasPrice: 0.1,
			expectFee:   cosmossdk_io_math.NewIntFromUint64(10000),
			expectError: false,
			epsilon:     cosmossdk_io_math.NewIntFromUint64(0),
		},
		{
			name:        "Zero gas",
			gas:         0,
			minGasPrice: 0.1,
			expectFee:   cosmossdk_io_math.NewInt(0),
			expectError: true,
			epsilon:     cosmossdk_io_math.NewIntFromUint64(0),
		},
		{
			name:        "Zero gas price",
			gas:         100000,
			minGasPrice: 0,
			expectFee:   cosmossdk_io_math.NewInt(0),
			expectError: true,
			epsilon:     cosmossdk_io_math.NewIntFromUint64(0),
		},
		{
			name:        "Negative gas price",
			gas:         100000,
			minGasPrice: -0.1,
			expectFee:   cosmossdk_io_math.NewInt(0),
			expectError: true,
			epsilon:     cosmossdk_io_math.NewIntFromUint64(0),
		},
		{
			name:        "Very small gas price",
			gas:         100000,
			minGasPrice: 0.000000001,
			expectFee:   cosmossdk_io_math.NewInt(0),
			expectError: false,
			epsilon:     cosmossdk_io_math.NewIntFromUint64(0),
		},
		{
			name:        "Very large gas",
			gas:         math.MaxUint64,
			minGasPrice: 0.1,
			expectFee:   cosmossdk_io_math.NewIntFromUint64(1844674407370955161), // MaxUint64 * 0.1
			expectError: false,
			epsilon:     cosmossdk_io_math.NewIntFromUint64(1000),
		},
		{
			name:        "Precise decimal gas price",
			gas:         100000,
			minGasPrice: 0.123456789,
			expectFee:   cosmossdk_io_math.NewIntFromUint64(12345),
			expectError: false,
			epsilon:     cosmossdk_io_math.NewIntFromUint64(1),
		},
		{
			name:        "Both large values",
			gas:         math.MaxUint64,
			minGasPrice: math.MaxFloat64,
			expectFee:   cosmossdk_io_math.NewIntFromBigInt(big.NewInt(math.MaxInt64)),
			expectError: true,
			epsilon:     cosmossdk_io_math.NewIntFromUint64(1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fee, err := CalculateFees(tt.gas, tt.minGasPrice)
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.True(t, tt.expectFee.Sub(fee).LTE(tt.epsilon))
		})
	}
}

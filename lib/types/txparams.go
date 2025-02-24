package transaction

import (
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
)

type TransactionParams struct {
	ChainID             string
	Denom               string
	Prefix              string
	Sequence            uint64
	AccNum              uint64
	PrivKey             cryptotypes.PrivKey
	PubKey              cryptotypes.PubKey
	TimeoutHeight       uint64
	GasEstimationConfig GasEstimationConfig
}

// GasEstimationConfig holds the parameters used for gas estimation.
type GasEstimationConfig struct {
	BaseGas       uint64  // Base gas cost for any transaction
	GasPerByte    uint64  // Gas cost per byte of transaction size
	MinGasPrice   float64 // Minimum gas price in the smallest denomination
	OverrideFees  uint64  // Override the gas price with a fixed value
	SimulateTx    bool    // if true, simulate the transaction via chain, using gasAdjustment
	GasAdjustment float64 // Adjustment factor for the gas used
	OverrideGas   uint64  // Override gas used
}

package usecase

import (
	errorsmod "cosmossdk.io/errors"
)

const ERROR_CODESPACE = "allora-offchain-usecase"

var (
	ErrAllNodesExhausted = errorsmod.Register(ERROR_CODESPACE, 1, "all available nodes have been tried and exhausted")
)

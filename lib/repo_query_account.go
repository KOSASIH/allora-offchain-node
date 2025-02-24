package lib

import (
	"context"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/types/query"
	auth "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/rs/zerolog/log"
)

// GetAccountInfo queries the account info from the chain (sequence number, account number)
func (node *NodeConfig) GetAccountInfo(ctx context.Context, inAddress string) (address string, sequence uint64, accNum uint64, err error) {
	walletConfig, err := node.ConnectionManager.GetWalletConfig()
	if err != nil {
		return "", 0, 0, errorsmod.Wrapf(err, "Error getting wallet config")
	}
	log.Info().Msgf("Getting account info for %s", inAddress)
	resp, err := QueryDataWithRetry(
		ctx,
		walletConfig.MaxRetries,
		walletConfig.RetryDelay,
		func(ctx context.Context, req query.PageRequest) (*auth.QueryAccountInfoResponse, error) {
			return node.Chain.AuthQueryClient.AccountInfo(ctx, &auth.QueryAccountInfoRequest{Address: inAddress})
		},
		query.PageRequest{}, // nolint:exhaustruct
		"get account info",
		node,
	)
	if err != nil {
		return "", 0, 0, err
	}

	if resp.Info == nil {
		return "", 0, 0, fmt.Errorf("account info is nil")
	}

	address = resp.Info.Address
	sequence = resp.Info.Sequence
	accNum = resp.Info.AccountNumber

	return address, sequence, accNum, nil
}

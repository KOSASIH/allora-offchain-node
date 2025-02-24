package auth

import (
	"sync"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
)

var (
	keyringCodec     codec.Codec
	keyringCodecOnce sync.Once
)

// GetKeyringCodec returns a singleton instance of the keyring codec
func GetKeyringCodec() codec.Codec {
	keyringCodecOnce.Do(func() {
		registry := codectypes.NewInterfaceRegistry()
		cryptocodec.RegisterInterfaces(registry)
		keyringCodec = codec.NewProtoCodec(registry)
	})
	return keyringCodec
}

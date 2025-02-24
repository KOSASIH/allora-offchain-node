package transaction

import (
	"sync"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
)

var (
	txCodec            codec.Codec
	txCodecOnce        sync.Once
	encodingConfig     moduletestutil.TestEncodingConfig
	encodingConfigOnce sync.Once
)

// GetTransactionCodec returns a singleton instance of the transaction codec
func GetTransactionCodec() codec.Codec {
	txCodecOnce.Do(func() {
		txCodec = codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	})
	return txCodec
}

func GetEncodingConfig() moduletestutil.TestEncodingConfig {
	encodingConfigOnce.Do(func() {
		txCodec = GetTransactionCodec()
		encodingConfig = moduletestutil.MakeTestEncodingConfig()
		encodingConfig.Codec = txCodec

	})
	return encodingConfig
}

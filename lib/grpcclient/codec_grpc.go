package grpcclient

import (
	"fmt"

	proto "github.com/cosmos/gogoproto/proto"
	// "google.golang.org/grpc/encoding"
)

// This custom codec provides a wrapper that works around issue of bad marshalling of sdk types
// In particular cosmossdk.io/math.Int , removing parentCodec
// Reference: https://github.com/cosmos/cosmos-sdk/issues/18430#issuecomment-2359148807
type customCodec struct{}

func (c customCodec) Marshal(v interface{}) ([]byte, error) {
	protoMsg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to assert proto.Message")
	}
	return proto.Marshal(protoMsg)
}

func (c customCodec) Unmarshal(data []byte, v interface{}) error {
	protoMsg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("failed to assert proto.Message")
	}
	return proto.Unmarshal(data, protoMsg)
}

func (c customCodec) Name() string {
	return "gogoproto"
}

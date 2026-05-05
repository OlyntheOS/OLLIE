//go:build !protobuf

package stream

import "errors"

// ProtoCodec is unavailable unless built with -tags=protobuf and generated code.
type ProtoCodec struct{}

func (ProtoCodec) Marshal(_ any) ([]byte, error) {
	return nil, errors.New("protobuf codec not enabled; build with -tags=protobuf")
}

func (ProtoCodec) Unmarshal(_ []byte, _ any) error {
	return errors.New("protobuf codec not enabled; build with -tags=protobuf")
}

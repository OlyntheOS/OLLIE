package stream

import "encoding/json"

// Codec encodes and decodes envelopes.
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// JSONCodec implements Codec with JSON.
type JSONCodec struct{}

func (JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

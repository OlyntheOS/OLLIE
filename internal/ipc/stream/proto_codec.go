//go:build protobuf

package stream

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	streampb "lumin-engine/internal/ipc/stream/pb"
)

// ProtoCodec encodes envelopes with protobuf and payloads with JSON.
type ProtoCodec struct {
	json JSONCodec
}

func (c ProtoCodec) Marshal(v any) ([]byte, error) {
	switch value := v.(type) {
	case Envelope:
		return c.marshalEnvelope(&value)
	case *Envelope:
		if value == nil {
			return nil, fmt.Errorf("nil envelope")
		}
		return c.marshalEnvelope(value)
	default:
		return c.json.Marshal(v)
	}
}

func (c ProtoCodec) Unmarshal(data []byte, v any) error {
	switch target := v.(type) {
	case *Envelope:
		if target == nil {
			return fmt.Errorf("nil envelope")
		}
		var pbEnv streampb.Envelope
		if err := proto.Unmarshal(data, &pbEnv); err != nil {
			return err
		}
		*target = fromProtoEnvelope(&pbEnv)
		return nil
	default:
		return c.json.Unmarshal(data, v)
	}
}

func (c ProtoCodec) marshalEnvelope(env *Envelope) ([]byte, error) {
	pbEnv := toProtoEnvelope(env)
	return proto.Marshal(pbEnv)
}

func toProtoEnvelope(env *Envelope) *streampb.Envelope {
	pbEnv := &streampb.Envelope{
		Version:   uint32(env.Version),
		Type:      streampb.MessageType(env.Type),
		RequestId: env.RequestID,
		SessionId: env.SessionID,
		TraceId:   env.TraceID,
		Payload:   env.Payload,
		Flags:     env.Flags,
		Timestamp: env.Timestamp,
	}
	if env.Error != nil {
		pbEnv.Error = &streampb.Error{
			Code:      env.Error.Code,
			Message:   env.Error.Message,
			Retryable: env.Error.Retryable,
		}
	}
	return pbEnv
}

func fromProtoEnvelope(pbEnv *streampb.Envelope) Envelope {
	env := Envelope{
		Version:   uint16(pbEnv.GetVersion()),
		Type:      MessageType(pbEnv.GetType()),
		RequestID: pbEnv.GetRequestId(),
		SessionID: pbEnv.GetSessionId(),
		TraceID:   pbEnv.GetTraceId(),
		Payload:   pbEnv.GetPayload(),
		Flags:     pbEnv.GetFlags(),
		Timestamp: pbEnv.GetTimestamp(),
	}
	if pbEnv.Error != nil {
		env.Error = &Error{
			Code:      pbEnv.Error.Code,
			Message:   pbEnv.Error.Message,
			Retryable: pbEnv.Error.Retryable,
		}
	}
	return env
}

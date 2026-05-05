//go:build protobuf

package stream

import (
	"encoding/json"
	"testing"
)

func TestProtoCodecEnvelopeRoundTrip(t *testing.T) {
	codec := ProtoCodec{}
	payload, err := json.Marshal(map[string]string{"ping": "pong"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	env := Envelope{
		Version:   CurrentVersion,
		Type:      MessageTypeRequest,
		RequestID: "req-1",
		SessionID: "sess-1",
		TraceID:   "trace-1",
		Payload:   payload,
		Error:     &Error{Code: "ok", Message: "", Retryable: false},
		Flags:     42,
		Timestamp: 123,
	}

	data, err := codec.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	var decoded Envelope
	if err := codec.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if decoded.RequestID != env.RequestID || decoded.SessionID != env.SessionID {
		t.Fatalf("id mismatch: %v", decoded)
	}
	if string(decoded.Payload) != string(env.Payload) {
		t.Fatalf("payload mismatch: %s", string(decoded.Payload))
	}
	if decoded.Error == nil || decoded.Error.Code != "ok" {
		t.Fatalf("error mismatch: %v", decoded.Error)
	}
}

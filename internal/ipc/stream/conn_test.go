package stream

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestConnEnvelopeRoundTrip(t *testing.T) {
	buf := &bytes.Buffer{}
	conn := NewConn(buf, buf, JSONCodec{}, 1024)
	payload, err := json.Marshal(map[string]string{"msg": "hi"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	env := Envelope{
		Type:      MessageTypeEvent,
		RequestID: "req-1",
		SessionID: "sess-1",
		Payload:   payload,
	}

	if err := conn.WriteEnvelope(env); err != nil {
		t.Fatalf("write envelope: %v", err)
	}

	got, err := conn.ReadEnvelope()
	if err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	if got.Version != CurrentVersion {
		t.Fatalf("version mismatch: got %d", got.Version)
	}
	if got.Type != env.Type {
		t.Fatalf("type mismatch: got %d", got.Type)
	}
	if got.RequestID != env.RequestID || got.SessionID != env.SessionID {
		t.Fatalf("id mismatch: got %q/%q", got.RequestID, got.SessionID)
	}
	if !bytes.Equal(got.Payload, env.Payload) {
		t.Fatalf("payload mismatch: got %s want %s", string(got.Payload), string(env.Payload))
	}
	if got.Timestamp == 0 {
		t.Fatalf("expected timestamp")
	}
}

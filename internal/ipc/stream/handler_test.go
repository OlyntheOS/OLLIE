package stream

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

type fakeBackend struct {
	generate string
	err      error
}

func (f fakeBackend) Generate(prompt string, maxTokens int) (string, error) {
	_, _ = prompt, maxTokens
	return f.generate, f.err
}

func (f fakeBackend) Health() map[string]any {
	return map[string]any{"status": "ok"}
}

func (f fakeBackend) LoadModel(path string) error {
	_ = path
	return nil
}

func (f fakeBackend) UnloadModel() error {
	return nil
}

func (f fakeBackend) Tool(name string, args []byte) (any, error) {
	_, _ = name, args
	return nil, nil
}

func TestHandlerHandshake(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	handler := NewHandler(fakeBackend{})
	go handler.HandleConn(ctx, serverConn)

	conn := NewConn(clientConn, clientConn, JSONCodec{}, 1024)
	payload, err := json.Marshal(Handshake{Protocol: ProtocolName, Version: CurrentVersion, Client: "test"})
	if err != nil {
		t.Fatalf("marshal handshake: %v", err)
	}

	env := Envelope{
		Type:      MessageTypeHandshake,
		RequestID: "hs-1",
		Payload:   payload,
	}
	if err := conn.WriteEnvelope(env); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	resp, err := conn.ReadEnvelope()
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.Type != MessageTypeResponse {
		t.Fatalf("unexpected response type: %v", resp.Type)
	}

	var ack HandshakeAck
	if err := json.Unmarshal(resp.Payload, &ack); err != nil {
		t.Fatalf("unmarshal handshake ack: %v", err)
	}
	if ack.Protocol != ProtocolName || ack.Version != CurrentVersion {
		t.Fatalf("unexpected ack: %v", ack)
	}
}

func TestHandlerStreamGenerate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	handler := NewHandler(fakeBackend{generate: "hello world"})
	go handler.HandleConn(ctx, serverConn)

	conn := NewConn(clientConn, clientConn, JSONCodec{}, 1024)
	reqPayload, err := json.Marshal(Request{
		Method: "generate",
		Stream: true,
		Params: mustMarshal(JSONCodec{}, map[string]any{"prompt": "hi", "max_tokens": 3}),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	if err := conn.WriteEnvelope(Envelope{Type: MessageTypeRequest, RequestID: "req-1", Payload: reqPayload}); err != nil {
		t.Fatalf("write request: %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var events []Event
	var response Envelope
	for i := 0; i < 6; i++ {
		env, err := conn.ReadEnvelope()
		if err != nil {
			t.Fatalf("read envelope: %v", err)
		}
		switch env.Type {
		case MessageTypeEvent:
			var ev Event
			if err := json.Unmarshal(env.Payload, &ev); err != nil {
				t.Fatalf("unmarshal event: %v", err)
			}
			events = append(events, ev)
		case MessageTypeResponse:
			response = env
			i = 6
		default:
			t.Fatalf("unexpected envelope type: %v", env.Type)
		}
	}

	if response.Type != MessageTypeResponse {
		t.Fatalf("missing response")
	}

	var tokenSeen, doneSeen bool
	for _, ev := range events {
		if ev.Type == "token" {
			tokenSeen = true
		}
		if ev.Type == "done" {
			doneSeen = true
		}
	}
	if !tokenSeen || !doneSeen {
		t.Fatalf("expected token and done events, got %v", events)
	}

	var result map[string]string
	if err := json.Unmarshal(response.Payload, &result); err != nil {
		t.Fatalf("unmarshal response payload: %v", err)
	}
	if result["text"] != "hello world" {
		t.Fatalf("unexpected response text: %q", result["text"])
	}
}

func TestHandlerCancelTracking(t *testing.T) {
	handler := NewHandler(fakeBackend{})
	ctx, cancel := context.WithCancel(context.Background())

	if !handler.trackRequest("req-1", cancel) {
		t.Fatalf("expected trackRequest to succeed")
	}
	if handler.trackRequest("req-1", cancel) {
		t.Fatalf("expected duplicate trackRequest to fail")
	}
	if !handler.cancelRequest("req-1") {
		t.Fatalf("expected cancelRequest to succeed")
	}
	select {
	case <-ctx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected context to be cancelled")
	}
	if handler.cancelRequest("req-1") {
		t.Fatalf("expected cancelRequest to fail for missing id")
	}
}

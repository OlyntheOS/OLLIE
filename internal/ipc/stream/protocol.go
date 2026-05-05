package stream

import "encoding/json"

const CurrentVersion uint16 = 1
const ProtocolName = "ollie-stream"

// MessageType tags the envelope payload.
type MessageType uint16

const (
	MessageTypeHandshake MessageType = 1
	MessageTypeRequest   MessageType = 2
	MessageTypeResponse  MessageType = 3
	MessageTypeEvent     MessageType = 4
	MessageTypeCancel    MessageType = 5
	MessageTypePing      MessageType = 6
	MessageTypePong      MessageType = 7
)

// Envelope is the framed unit exchanged on the stream.
type Envelope struct {
	Version   uint16          `json:"v"`
	Type      MessageType     `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	TraceID   string          `json:"trace_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *Error          `json:"error,omitempty"`
	Flags     uint32          `json:"flags,omitempty"`
	Timestamp int64           `json:"ts,omitempty"`
}

// Handshake is the client-side protocol hello.
type Handshake struct {
	Protocol string `json:"protocol"`
	Version  uint16 `json:"version"`
	Client   string `json:"client,omitempty"`
}

// HandshakeAck confirms protocol compatibility.
type HandshakeAck struct {
	Protocol string `json:"protocol"`
	Version  uint16 `json:"version"`
	Server   string `json:"server,omitempty"`
}

// Error is a structured transport or request error.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

// Request is an RPC-style request over the stream protocol.
type Request struct {
	Method    string            `json:"method"`
	Params    json.RawMessage   `json:"params,omitempty"`
	TimeoutMs int               `json:"timeout_ms,omitempty"`
	Stream    bool              `json:"stream,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Response carries the request result.
type Response struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Event is a streamed message tied to a request.
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

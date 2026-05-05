package stream

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"lumin-engine/internal/agent"
	"lumin-engine/internal/observability"
	"lumin-engine/internal/tools"
)

const (
	errCodeDecode      = "decode_error"
	errCodeInvalid     = "invalid_request"
	errCodeUnsupported = "unsupported"
	errCodeInternal    = "internal_error"
)

// Backend is the engine surface for IPC.
type Backend interface {
	Generate(prompt string, maxTokens int) (string, error)
	Health() map[string]any
	LoadModel(path string) error
	UnloadModel() error
	Tool(name string, args []byte) (any, error)
}

// AgentBackend exposes the agent runtime.
type AgentBackend interface {
	RunAgent(ctx context.Context, message string, out chan<- agent.Event)
}

// Handler serves the framed stream protocol.
type Handler struct {
	backend       Backend
	codec         Codec
	maxFrameSize  int
	serverName    string
	allowHandshake bool
	activeMu      sync.Mutex
	active        map[string]context.CancelFunc
	metrics       *observability.Metrics
}

func NewHandler(backend Backend) *Handler {
	return &Handler{
		backend:       backend,
		codec:         JSONCodec{},
		maxFrameSize:  DefaultMaxFrameSize,
		serverName:    "lumin-engine",
		allowHandshake: true,
		active:        make(map[string]context.CancelFunc),
	}
}

func (h *Handler) WithCodec(codec Codec) *Handler {
	if codec != nil {
		h.codec = codec
	}
	return h
}

func (h *Handler) WithMaxFrameSize(maxSize int) *Handler {
	if maxSize > 0 {
		h.maxFrameSize = maxSize
	}
	return h
}

func (h *Handler) WithServerName(name string) *Handler {
	if name != "" {
		h.serverName = name
	}
	return h
}

func (h *Handler) WithMetrics(metrics *observability.Metrics) *Handler {
	h.metrics = metrics
	return h
}

func (h *Handler) HandleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	streamConn := NewConn(conn, conn, h.codec, h.maxFrameSize)

	for {
		env, err := streamConn.ReadEnvelope()
		if err != nil {
			return
		}
		if env.Version != 0 && env.Version != CurrentVersion {
			h.replyError(streamConn, env, errCodeUnsupported, "unsupported protocol version", false)
			continue
		}

		switch env.Type {
		case MessageTypeHandshake:
			if !h.allowHandshake {
				h.replyError(streamConn, env, errCodeUnsupported, "handshake not supported", false)
				continue
			}
			h.handleHandshake(streamConn, env)
		case MessageTypePing:
			h.replyEnvelope(streamConn, Envelope{
				Type:      MessageTypePong,
				RequestID: env.RequestID,
				SessionID: env.SessionID,
				TraceID:   env.TraceID,
			})
		case MessageTypeCancel:
			h.handleCancel(streamConn, env)
		case MessageTypeRequest:
			if env.RequestID == "" {
				h.replyError(streamConn, env, errCodeInvalid, "missing request_id", false)
				continue
			}
			if h.metrics != nil {
				h.metrics.IncRequests()
			}
			h.handleRequest(ctx, streamConn, env)
		default:
			h.replyError(streamConn, env, errCodeInvalid, "unknown message type", false)
		}
	}
}

func (h *Handler) handleHandshake(conn *Conn, env Envelope) {
	var hs Handshake
	if len(env.Payload) > 0 {
		if err := h.codec.Unmarshal(env.Payload, &hs); err != nil {
			h.replyError(conn, env, errCodeDecode, "invalid handshake payload", false)
			return
		}
	}

	if hs.Protocol != "" && hs.Protocol != ProtocolName {
		h.replyError(conn, env, errCodeUnsupported, "protocol mismatch", false)
		return
	}

	ack := HandshakeAck{Protocol: ProtocolName, Version: CurrentVersion, Server: h.serverName}
	payload, err := h.codec.Marshal(ack)
	if err != nil {
		h.replyError(conn, env, errCodeInternal, "handshake encode failed", false)
		return
	}
	resp := Envelope{
		Type:      MessageTypeResponse,
		RequestID: env.RequestID,
		SessionID: env.SessionID,
		TraceID:   env.TraceID,
		Payload:   payload,
	}
	_ = conn.WriteEnvelope(resp)
}

func (h *Handler) handleRequest(ctx context.Context, conn *Conn, env Envelope) {
	var req Request
	if err := h.codec.Unmarshal(env.Payload, &req); err != nil {
		h.replyError(conn, env, errCodeDecode, "invalid request payload", false)
		return
	}
	if req.Stream {
		h.handleStreamRequest(ctx, conn, env, req)
		return
	}

	if req.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	result, err := h.dispatch(ctx, req)
	if err != nil {
		h.replyError(conn, env, errCodeInternal, err.Error(), false)
		return
	}
	payload, err := h.codec.Marshal(result)
	if err != nil {
		h.replyError(conn, env, errCodeInternal, "encode result failed", false)
		return
	}

	resp := Envelope{
		Type:      MessageTypeResponse,
		RequestID: env.RequestID,
		SessionID: env.SessionID,
		TraceID:   env.TraceID,
		Payload:   payload,
	}
	_ = conn.WriteEnvelope(resp)
}

func (h *Handler) dispatch(ctx context.Context, req Request) (any, error) {
	_ = ctx
	if req.Method == "" {
		return nil, errors.New("missing method")
	}

	switch req.Method {
	case "health":
		return h.backend.Health(), nil
	case "generate":
		var request struct {
			Prompt    string `json:"prompt"`
			MaxTokens int    `json:"max_tokens"`
		}
		if err := json.Unmarshal(req.Params, &request); err != nil {
			return nil, err
		}
		return h.backend.Generate(request.Prompt, request.MaxTokens)
	case "model.load":
		var request struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(req.Params, &request); err != nil {
			return nil, err
		}
		return nil, h.backend.LoadModel(request.Path)
	case "model.unload":
		return nil, h.backend.UnloadModel()
	case "tool.call":
		var request tools.Call
		if err := json.Unmarshal(req.Params, &request); err != nil {
			return nil, err
		}
		return h.backend.Tool(request.Name, request.Arguments)
	default:
		return nil, errors.New("unknown method: " + req.Method)
	}
}

func (h *Handler) handleStreamRequest(ctx context.Context, conn *Conn, env Envelope, req Request) {
	if req.Method == "" {
		h.replyError(conn, env, errCodeInvalid, "missing method", false)
		return
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if req.TimeoutMs > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMs)*time.Millisecond)
	} else {
		requestCtx, cancel = context.WithCancel(ctx)
	}

	if !h.trackRequest(env.RequestID, cancel) {
		h.replyError(conn, env, errCodeInvalid, "duplicate request_id", false)
		cancel()
		return
	}

	go func() {
		defer h.untrackRequest(env.RequestID)
		defer cancel()

		switch req.Method {
		case "generate":
			h.handleStreamGenerate(requestCtx, conn, env, req)
		case "agent.run":
			h.handleStreamAgentRun(requestCtx, conn, env, req)
		default:
			h.replyError(conn, env, errCodeUnsupported, "streaming method not supported", false)
		}
	}()
}

func (h *Handler) handleStreamAgentRun(ctx context.Context, conn *Conn, env Envelope, req Request) {
		agentBackend, ok := h.backend.(AgentBackend)
		if !ok {
			h.replyError(conn, env, errCodeUnsupported, "agent runtime not available", false)
			return
		}

		var request struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(req.Params, &request); err != nil {
			h.replyError(conn, env, errCodeDecode, "invalid agent.run payload", false)
			return
		}
		if strings.TrimSpace(request.Message) == "" {
			h.replyError(conn, env, errCodeInvalid, "missing message", false)
			return
		}

		eventCh := make(chan agent.Event, 16)
		go agentBackend.RunAgent(ctx, request.Message, eventCh)

		var output strings.Builder
		var runErr error
		for ev := range eventCh {
			switch ev.Type {
			case agent.EventToken:
				output.WriteString(ev.Text)
				h.sendEvent(conn, env, Event{Type: "token", Data: mustMarshal(h.codec, map[string]string{"text": ev.Text})})
			case agent.EventToolCall:
				if h.metrics != nil {
					h.metrics.IncToolCalls()
				}
				h.sendEvent(conn, env, Event{Type: "tool_call", Data: mustMarshal(h.codec, ev.ToolCall)})
			case agent.EventToolResult:
				h.sendEvent(conn, env, Event{Type: "tool_result", Data: mustMarshal(h.codec, ev.ToolResult)})
			case agent.EventError:
				runErr = ev.Err
				h.sendEvent(conn, env, Event{Type: "error", Data: mustMarshal(h.codec, map[string]string{"error": ev.Err.Error()})})
			case agent.EventDone:
				h.sendEvent(conn, env, Event{Type: "done", Data: mustMarshal(h.codec, map[string]string{"status": "done"})})
			}
		}

		if runErr != nil {
			h.replyError(conn, env, errCodeInternal, runErr.Error(), false)
			return
		}

		payload, err := h.codec.Marshal(map[string]string{"text": output.String()})
		if err != nil {
			h.replyError(conn, env, errCodeInternal, "encode result failed", false)
			return
		}

		resp := Envelope{
			Type:      MessageTypeResponse,
			RequestID: env.RequestID,
			SessionID: env.SessionID,
			TraceID:   env.TraceID,
			Payload:   payload,
		}
		_ = conn.WriteEnvelope(resp)
	}

func (h *Handler) handleStreamGenerate(ctx context.Context, conn *Conn, env Envelope, req Request) {
	var request struct {
		Prompt    string `json:"prompt"`
		MaxTokens int    `json:"max_tokens"`
	}
	if err := json.Unmarshal(req.Params, &request); err != nil {
		h.replyError(conn, env, errCodeDecode, "invalid generate payload", false)
		return
	}

	output, err := h.backend.Generate(request.Prompt, request.MaxTokens)
	if err != nil {
		h.sendStreamError(conn, env, err)
		return
	}

	for _, chunk := range chunkText(output, 64) {
		if ctx.Err() != nil {
			h.sendStreamError(conn, env, ctx.Err())
			return
		}
		h.sendEvent(conn, env, Event{Type: "token", Data: mustMarshal(h.codec, map[string]string{"text": chunk})})
	}

	if ctx.Err() != nil {
		h.sendStreamError(conn, env, ctx.Err())
		return
	}

	endData := mustMarshal(h.codec, map[string]string{"status": "done"})
	h.sendEvent(conn, env, Event{Type: "done", Data: endData})

	payload, err := h.codec.Marshal(map[string]string{"text": output})
	if err != nil {
		h.replyError(conn, env, errCodeInternal, "encode result failed", false)
		return
	}

	resp := Envelope{
		Type:      MessageTypeResponse,
		RequestID: env.RequestID,
		SessionID: env.SessionID,
		TraceID:   env.TraceID,
		Payload:   payload,
	}
	_ = conn.WriteEnvelope(resp)
}

func (h *Handler) handleCancel(conn *Conn, env Envelope) {
	if env.RequestID == "" {
		h.replyError(conn, env, errCodeInvalid, "missing request_id", false)
		return
	}
	if h.cancelRequest(env.RequestID) {
		return
	}
	h.replyError(conn, env, errCodeInvalid, "unknown request_id", false)
}

func (h *Handler) trackRequest(id string, cancel context.CancelFunc) bool {
	h.activeMu.Lock()
	defer h.activeMu.Unlock()
	if _, exists := h.active[id]; exists {
		return false
	}
	h.active[id] = cancel
	return true
}

func (h *Handler) untrackRequest(id string) {
	h.activeMu.Lock()
	defer h.activeMu.Unlock()
	delete(h.active, id)
}

func (h *Handler) cancelRequest(id string) bool {
	h.activeMu.Lock()
	cancel, ok := h.active[id]
	if ok {
		delete(h.active, id)
	}
	h.activeMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (h *Handler) sendStreamError(conn *Conn, env Envelope, err error) {
	data := mustMarshal(h.codec, map[string]string{"error": err.Error()})
	h.sendEvent(conn, env, Event{Type: "error", Data: data})
	h.replyError(conn, env, errCodeInternal, err.Error(), false)
}

func (h *Handler) sendEvent(conn *Conn, env Envelope, event Event) {
	payload, err := h.codec.Marshal(event)
	if err != nil {
		return
	}
	if h.metrics != nil {
		h.metrics.IncEvents()
	}
	envelope := Envelope{
		Type:      MessageTypeEvent,
		RequestID: env.RequestID,
		SessionID: env.SessionID,
		TraceID:   env.TraceID,
		Payload:   payload,
	}
	_ = conn.WriteEnvelope(envelope)
}

func mustMarshal(codec Codec, value any) json.RawMessage {
	data, err := codec.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func chunkText(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = 64
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		if len(runes) == 0 {
			return nil
		}
		return []string{text}
	}
	chunks := make([]string, 0, (len(runes)/maxRunes)+1)
	for i := 0; i < len(runes); i += maxRunes {
		end := i + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

func (h *Handler) replyError(conn *Conn, env Envelope, code, message string, retryable bool) {
	resp := Envelope{
		Type:      MessageTypeResponse,
		RequestID: env.RequestID,
		SessionID: env.SessionID,
		TraceID:   env.TraceID,
		Error: &Error{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	}
	if h.metrics != nil {
		h.metrics.IncErrors()
	}
	_ = conn.WriteEnvelope(resp)
}

func (h *Handler) replyEnvelope(conn *Conn, env Envelope) {
	_ = conn.WriteEnvelope(env)
}

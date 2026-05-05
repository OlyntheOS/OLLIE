package observability

import "sync/atomic"

// Metrics holds simple runtime counters.
type Metrics struct {
	requests uint64
	errors   uint64
	toolCalls uint64
	events   uint64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) IncRequests() {
	atomic.AddUint64(&m.requests, 1)
}

func (m *Metrics) IncErrors() {
	atomic.AddUint64(&m.errors, 1)
}

func (m *Metrics) IncToolCalls() {
	atomic.AddUint64(&m.toolCalls, 1)
}

func (m *Metrics) IncEvents() {
	atomic.AddUint64(&m.events, 1)
}

func (m *Metrics) Snapshot() map[string]any {
	return map[string]any{
		"requests":  atomic.LoadUint64(&m.requests),
		"errors":    atomic.LoadUint64(&m.errors),
		"tool_calls": atomic.LoadUint64(&m.toolCalls),
		"events":    atomic.LoadUint64(&m.events),
	}
}

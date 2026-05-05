package context

import "strings"

// Manager keeps a token-budgeted message history.
type Manager struct {
	MaxTokens    int
	Messages     []Message
	tokenCounter func(string) int
	totalTokens  int
}

func NewManager(maxTokens int, tokenCounter func(string) int) *Manager {
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	if tokenCounter == nil {
		tokenCounter = defaultTokenCounter
	}
	return &Manager{MaxTokens: maxTokens, tokenCounter: tokenCounter}
}

func (m *Manager) Add(msg Message) {
	if msg.TokenCount <= 0 {
		msg.TokenCount = m.tokenCounter(msg.Content)
	}
	m.Messages = append(m.Messages, msg)
	m.totalTokens += msg.TokenCount
	m.Trim()
}

func (m *Manager) Count() int {
	return m.totalTokens
}

func (m *Manager) Reset() {
	m.Messages = nil
	m.totalTokens = 0
}

func (m *Manager) Trim() {
	for m.totalTokens > m.MaxTokens {
		if !m.DropOldestNonSystem() {
			return
		}
	}
}

func (m *Manager) DropOldestNonSystem() bool {
	for i, msg := range m.Messages {
		if strings.EqualFold(msg.Role, "system") {
			continue
		}
		m.totalTokens -= msg.TokenCount
		m.Messages = append(m.Messages[:i], m.Messages[i+1:]...)
		if m.totalTokens < 0 {
			m.totalTokens = 0
		}
		return true
	}
	return false
}

func defaultTokenCounter(text string) int {
	return len(strings.Fields(text))
}

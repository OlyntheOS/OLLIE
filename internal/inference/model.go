package inference

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type Model struct {
	mu                sync.RWMutex
	genMu             sync.Mutex
	Path              string
	Name              string
	MaxContextTokens  int
	loaded            bool
	suspended         bool
	backend           *LlamaModel
	context           *LlamaContext
}

func NewModel(path string, maxContextTokens int) *Model {
	name := "default"
	if path != "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if maxContextTokens <= 0 {
		maxContextTokens = 8192
	}
	return &Model{Path: path, Name: name, MaxContextTokens: maxContextTokens}
}

func (m *Model) Load(path string) error {
	m.genMu.Lock()
	defer m.genMu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.backend != nil {
		m.backend.Free()
		m.backend = nil
	}
	if m.context != nil {
		m.context.Free()
		m.context = nil
	}

	if path != "" {
		m.Path = path
		m.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if m.Path == "" {
		return fmt.Errorf("model path is empty")
	}

	backend, err := LoadModel(m.Path, 0)
	if err != nil {
		return err
	}

	context, err := backend.NewContext(m.MaxContextTokens)
	if err != nil {
		backend.Free()
		return err
	}

	m.backend = backend
	m.context = context
	m.loaded = true
	m.suspended = false
	return nil
}

func (m *Model) Unload() error {
	m.genMu.Lock()
	defer m.genMu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.context != nil {
		m.context.Free()
		m.context = nil
	}
	if m.backend != nil {
		m.backend.Free()
		m.backend = nil
	}
	m.loaded = false
	m.suspended = false
	return nil
}

func (m *Model) Suspend() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.loaded {
		return nil
	}
	m.suspended = true
	return nil
}

func (m *Model) EnsureLoaded() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.loaded || m.backend == nil || m.context == nil {
		return fmt.Errorf("model is not loaded")
	}
	if m.suspended {
		return fmt.Errorf("model is suspended")
	}
	return nil
}

func (m *Model) Generate(prompt string, maxTokens int) (string, error) {
	m.genMu.Lock()
	defer m.genMu.Unlock()

	if err := m.EnsureLoaded(); err != nil {
		return "", err
	}

	m.mu.RLock()
	backend := m.backend
	context := m.context
	m.mu.RUnlock()

	if backend == nil || context == nil {
		return "", fmt.Errorf("model is not loaded")
	}
	return backend.Generate(context, prompt, maxTokens)
}

func (m *Model) GenerateStream(ctx context.Context, tokens []int32, maxTokens int, out chan<- string) error {
	m.genMu.Lock()
	defer m.genMu.Unlock()

	if err := m.EnsureLoaded(); err != nil {
		return err
	}

	m.mu.RLock()
	backend := m.backend
	context := m.context
	m.mu.RUnlock()

	if backend == nil || context == nil {
		return fmt.Errorf("model is not loaded")
	}

	return backend.GenerateStream(ctx, context, tokens, maxTokens, func(token int32) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		text, err := backend.TokenToText(token)
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- text:
		}
		return nil
	})
}

func (m *Model) Tokenize(text string, addSpecial bool) ([]int32, error) {
	m.mu.RLock()
	backend := m.backend
	m.mu.RUnlock()
	if backend == nil {
		return nil, fmt.Errorf("model is not loaded")
	}
	return backend.Tokenize(text, addSpecial)
}

func (m *Model) Detokenize(tokens []int32) (string, error) {
	m.mu.RLock()
	backend := m.backend
	m.mu.RUnlock()
	if backend == nil {
		return "", fmt.Errorf("model is not loaded")
	}
	return backend.Detokenize(tokens)
}

func (m *Model) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]any{
		"name":      m.Name,
		"path":      m.Path,
		"context":   m.MaxContextTokens,
		"loaded":    m.loaded,
		"suspended": m.suspended,
	}
}

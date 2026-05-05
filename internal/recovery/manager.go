package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Manager persists session state for crash recovery.
type Manager struct {
	dir string
	mu  sync.Mutex
}

func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

func (m *Manager) SaveSession(id string, state any) error {
	if id == "" {
		return fmt.Errorf("session id is empty")
	}
	if m.dir == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	path := filepath.Join(m.dir, id+".json")
	return os.WriteFile(path, data, 0o644)
}

func (m *Manager) LoadSession(id string, out any) error {
	if id == "" {
		return fmt.Errorf("session id is empty")
	}
	if m.dir == "" {
		return fmt.Errorf("recovery path not configured")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (m *Manager) DeleteSession(id string) error {
	if id == "" {
		return fmt.Errorf("session id is empty")
	}
	if m.dir == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

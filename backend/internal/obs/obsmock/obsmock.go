// Package obsmock provides an in-memory fake of obs.Controller for tests.
package obsmock

import (
	"fmt"
	"sync"

	"live-orchestrator/backend/internal/obs"
)

var _ obs.Controller = (*Mock)(nil)

// Mock is a fake implementation of obs.Controller, safe for concurrent use.
// Inputs/Enabled/Muted let tests assert on OBS side effects without a live
// OBS connection.
type Mock struct {
	mu sync.Mutex

	Scenes  map[string]bool
	Inputs  map[string]string // inputName -> current source URL
	Enabled map[string]bool   // inputName -> scene item enabled state
	Muted   map[string]bool   // inputName -> audio muted state

	Connected      bool
	ReconnectCalls int
	ReconnectErr   error

	// CreatePositionInputErr, when set, is returned by the next
	// CreatePositionInput call instead of the normal success path.
	CreatePositionInputErr error
	// SetPositionEnabledErr, when set, is returned by every
	// SetPositionEnabled call instead of the normal success path.
	SetPositionEnabledErr error
	// SetInputAudioMutedErr, when set, is returned by every
	// SetInputAudioMuted call instead of the normal success path.
	SetInputAudioMutedErr error
}

// New creates a Mock with an initially connected state.
func New() *Mock {
	return &Mock{
		Scenes:    make(map[string]bool),
		Inputs:    make(map[string]string),
		Enabled:   make(map[string]bool),
		Muted:     make(map[string]bool),
		Connected: true,
	}
}

func (m *Mock) EnsureScene(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Scenes[name] = true
	return nil
}

func (m *Mock) CreatePositionInput(sceneName, inputName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CreatePositionInputErr != nil {
		return m.CreatePositionInputErr
	}
	if _, exists := m.Inputs[inputName]; exists {
		return fmt.Errorf("input %q already exists", inputName)
	}
	m.Inputs[inputName] = ""
	m.Enabled[inputName] = false
	return nil
}

func (m *Mock) UpdatePositionSource(inputName, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.Inputs[inputName]; !exists {
		return obs.ErrInputNotFound
	}
	m.Inputs[inputName] = url
	return nil
}

func (m *Mock) SetPositionEnabled(sceneName, inputName string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SetPositionEnabledErr != nil {
		return m.SetPositionEnabledErr
	}
	if _, exists := m.Inputs[inputName]; !exists {
		return obs.ErrInputNotFound
	}
	m.Enabled[inputName] = enabled
	return nil
}

func (m *Mock) SetInputAudioMuted(inputName string, muted bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SetInputAudioMutedErr != nil {
		return m.SetInputAudioMutedErr
	}
	m.Muted[inputName] = muted
	return nil
}

func (m *Mock) RemoveInput(inputName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Inputs, inputName)
	delete(m.Enabled, inputName)
	delete(m.Muted, inputName)
	return nil
}

func (m *Mock) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Connected
}

func (m *Mock) Reconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ReconnectCalls++
	if m.ReconnectErr == nil {
		m.Connected = true
	}
	return m.ReconnectErr
}

// Package obsmock provides an in-memory fake of obs.Controller for tests.
package obsmock

import (
	"sync"

	"live-orchestrator/backend/internal/obs"
)

var _ obs.Controller = (*Mock)(nil)

// Mock is a fake implementation of obs.Controller, safe for concurrent use.
type Mock struct {
	mu sync.Mutex

	Scenes         map[string]bool
	Inputs         map[string]string // inputName -> url
	VisibleInput   map[string]string // sceneName -> visible inputName
	Connected      bool
	ReconnectCalls int
	ReconnectErr   error
}

// New creates a Mock with an initially connected state.
func New() *Mock {
	return &Mock{
		Scenes:       make(map[string]bool),
		Inputs:       make(map[string]string),
		VisibleInput: make(map[string]string),
		Connected:    true,
	}
}

func (m *Mock) EnsureScene(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Scenes[name] = true
	return nil
}

func (m *Mock) CreateCameraInput(sceneName, inputName, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Inputs[inputName] = url
	return nil
}

func (m *Mock) RemoveInput(inputName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Inputs, inputName)
	for scene, visible := range m.VisibleInput {
		if visible == inputName {
			delete(m.VisibleInput, scene)
		}
	}
	return nil
}

func (m *Mock) SetOnlyVisibleSource(sceneName, inputName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.VisibleInput[sceneName] = inputName
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

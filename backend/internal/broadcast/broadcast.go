package broadcast

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/obs"
)

var (
	ErrNoActiveClient                = errors.New("no active client")
	ErrNoDestinationsConfigured      = errors.New("no destinations configured")
	ErrNothingLive                   = errors.New("nothing live")
	ErrClientSwitchWhileBroadcasting = errors.New("cannot switch client while broadcasting")
	ErrDestinationNotFound           = errors.New("destination not found")
	ErrBroadcastStopped              = errors.New("broadcast is stopped")
)

// Destination is an active live-id resolved to a push URL.
type Destination struct {
	LiveID       uuid.UUID
	PlatformName string
	PushURL      string
}

// LiveIDProvider is declared in the consumer package.
type LiveIDProvider interface {
	ListActiveForClient(ctx context.Context, clientID uuid.UUID) ([]Destination, error)
}

// LiveChecker reports whether the orchestrator currently has something on air.
type LiveChecker interface {
	SomethingLive() bool
}

// DestinationStatus is the per-platform relay status exposed via REST/WS.
type DestinationStatus struct {
	LiveID       uuid.UUID `json:"liveId"`
	PlatformName string    `json:"platformName"`
	State        string    `json:"state"`
	LastError    string    `json:"lastError"`
}

// StatusSnapshot is the full broadcast status payload.
type StatusSnapshot struct {
	ActiveClientID *uuid.UUID          `json:"activeClientId"`
	Running        bool                `json:"running"`
	Destinations   []DestinationStatus `json:"destinations"`
}

const (
	StateConnected = "connected"
	StateFailed    = "failed"
	StateStopped   = "stopped"
)

type destRuntime struct {
	status DestinationStatus
	proc   Process
	dest   Destination
	gen    uint64
}

// Manager owns active-client selection and one subprocess per destination.
type Manager struct {
	obsCtl           obs.Controller
	liveIDs          LiveIDProvider
	programStreamURL string
	runner           ProcessRunner
	hub              *events.Hub
	live             LiveChecker

	mu      sync.Mutex
	client  *uuid.UUID
	running bool
	dests   map[uuid.UUID]*destRuntime
	nextGen uint64
}

// NewManager constructs a Manager. hub and live may be nil (no WS / always "live" false).
func NewManager(
	obsCtl obs.Controller,
	liveIDs LiveIDProvider,
	programStreamURL string,
	runner ProcessRunner,
	hub *events.Hub,
	live LiveChecker,
) *Manager {
	return &Manager{
		obsCtl:           obsCtl,
		liveIDs:          liveIDs,
		programStreamURL: programStreamURL,
		runner:           runner,
		hub:              hub,
		live:             live,
		dests:            make(map[uuid.UUID]*destRuntime),
	}
}

// SetActiveClient selects the client whose destinations will be used on Start.
func (m *Manager) SetActiveClient(ctx context.Context, clientID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return ErrClientSwitchWhileBroadcasting
	}
	if m.client != nil && *m.client == clientID {
		return nil
	}
	id := clientID
	m.client = &id
	m.publishLocked()
	return nil
}

// ActiveClient returns the currently selected client, or nil.
func (m *Manager) ActiveClient() *uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		return nil
	}
	id := *m.client
	return &id
}

// Start begins OBS program stream and one ffmpeg process per destination.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	if m.client == nil {
		m.mu.Unlock()
		return ErrNoActiveClient
	}
	clientID := *m.client
	m.mu.Unlock()

	if m.live != nil && !m.live.SomethingLive() {
		return ErrNothingLive
	}

	dests, err := m.liveIDs.ListActiveForClient(ctx, clientID)
	if err != nil {
		return err
	}
	if len(dests) == 0 {
		return ErrNoDestinationsConfigured
	}

	if m.obsCtl != nil && !m.obsCtl.IsStreaming() {
		if err := m.obsCtl.StartProgramStream(m.programStreamURL); err != nil {
			return err
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil
	}
	if m.client == nil || *m.client != clientID {
		return ErrNoActiveClient
	}

	m.dests = make(map[uuid.UUID]*destRuntime)
	for _, d := range dests {
		if err := m.spawnLocked(ctx, d); err != nil {
			m.killAllLocked()
			m.dests = make(map[uuid.UUID]*destRuntime)
			if m.obsCtl != nil {
				_ = m.obsCtl.StopProgramStream()
			}
			return err
		}
	}
	m.running = true
	slog.Info("broadcast_started", "clientId", clientID.String(), "destinations", len(dests))
	m.publishLocked()
	return nil
}

// Stop kills every process and stops the OBS program stream.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return nil
	}
	m.killAllLocked()
	m.running = false
	if m.obsCtl != nil {
		_ = m.obsCtl.StopProgramStream()
	}
	clientID := ""
	if m.client != nil {
		clientID = m.client.String()
	}
	slog.Info("broadcast_stopped", "clientId", clientID)
	m.publishLocked()
	return nil
}

// Status returns a copy of per-destination statuses.
func (m *Manager) Status() []DestinationStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusSliceLocked()
}

// Snapshot returns the full broadcast status for REST/WS.
func (m *Manager) Snapshot() StatusSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

// Running reports whether Start has been called and not yet Stopped.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// RestartDestination kills and respawns a single destination's process.
func (m *Manager) RestartDestination(ctx context.Context, liveID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return ErrBroadcastStopped
	}
	rt, ok := m.dests[liveID]
	if !ok {
		return ErrDestinationNotFound
	}
	if rt.proc != nil {
		_ = rt.proc.Kill()
		rt.proc = nil
	}
	rt.gen++
	gen := rt.gen
	// Detach from the caller's context: the respawned relay is owned by the
	// Manager, not the request that triggered the restart (see spawnLocked).
	proc, err := m.runner.Start(context.WithoutCancel(ctx), m.programStreamURL, rt.dest.PushURL)
	if err != nil {
		rt.status.State = StateFailed
		rt.status.LastError = err.Error()
		m.publishLocked()
		return err
	}
	rt.proc = proc
	rt.status.State = StateConnected
	rt.status.LastError = ""
	slog.Info("destination_started", "platform", rt.dest.PlatformName, "liveId", liveID.String())
	go m.watch(liveID, gen, proc)
	m.publishLocked()
	return nil
}

func (m *Manager) spawnLocked(ctx context.Context, d Destination) error {
	// The relay process must outlive the caller's context (e.g. an HTTP request
	// that returns immediately after Start). Its lifetime is owned by the
	// Manager and ended only by Kill on Stop/RestartDestination/shutdown, so we
	// detach cancellation here to keep any context values but drop the deadline.
	proc, err := m.runner.Start(context.WithoutCancel(ctx), m.programStreamURL, d.PushURL)
	if err != nil {
		return err
	}
	m.nextGen++
	gen := m.nextGen
	rt := &destRuntime{
		status: DestinationStatus{
			LiveID:       d.LiveID,
			PlatformName: d.PlatformName,
			State:        StateConnected,
		},
		proc: proc,
		dest: d,
		gen:  gen,
	}
	m.dests[d.LiveID] = rt
	slog.Info("destination_started", "platform", d.PlatformName, "liveId", d.LiveID.String())
	go m.watch(d.LiveID, gen, proc)
	return nil
}

func (m *Manager) watch(liveID uuid.UUID, gen uint64, proc Process) {
	err := proc.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, ok := m.dests[liveID]
	if !ok || rt.gen != gen {
		return
	}
	if !m.running {
		return
	}
	rt.proc = nil
	rt.status.State = StateFailed
	if err != nil {
		rt.status.LastError = err.Error()
	} else {
		rt.status.LastError = "process exited"
	}
	slog.Info("destination_failed", "platform", rt.dest.PlatformName, "liveId", liveID.String(), "error", rt.status.LastError)
	m.publishLocked()
}

func (m *Manager) killAllLocked() {
	for _, rt := range m.dests {
		if rt.proc != nil {
			_ = rt.proc.Kill()
			rt.proc = nil
		}
		rt.gen++
		rt.status.State = StateStopped
		rt.status.LastError = ""
	}
}

func (m *Manager) statusSliceLocked() []DestinationStatus {
	out := make([]DestinationStatus, 0, len(m.dests))
	for _, rt := range m.dests {
		out = append(out, rt.status)
	}
	return out
}

func (m *Manager) snapshotLocked() StatusSnapshot {
	var client *uuid.UUID
	if m.client != nil {
		id := *m.client
		client = &id
	}
	dests := m.statusSliceLocked()
	if dests == nil {
		dests = []DestinationStatus{}
	}
	return StatusSnapshot{
		ActiveClientID: client,
		Running:        m.running,
		Destinations:   dests,
	}
}

func (m *Manager) publishLocked() {
	if m.hub == nil {
		return
	}
	m.hub.Publish(events.Event{Type: "broadcast.status", Payload: m.snapshotLocked()})
}

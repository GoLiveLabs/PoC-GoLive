package broadcast

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/obs/obsmock"
)

type fakeProcess struct {
	mu       sync.Mutex
	waitCh   chan error
	waitErr  error
	killed   bool
	killErr  error
	waitOnce sync.Once
}

func newFakeProcess() *fakeProcess {
	return &fakeProcess{waitCh: make(chan error, 1)}
}

func (p *fakeProcess) Wait() error {
	err, ok := <-p.waitCh
	if !ok {
		return p.waitErr
	}
	p.mu.Lock()
	p.waitErr = err
	p.mu.Unlock()
	return err
}

func (p *fakeProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.killed {
		return p.killErr
	}
	p.killed = true
	p.waitOnce.Do(func() {
		select {
		case p.waitCh <- nil:
		default:
		}
		close(p.waitCh)
	})
	return p.killErr
}

func (p *fakeProcess) exit(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.killed {
		return
	}
	p.waitOnce.Do(func() {
		select {
		case p.waitCh <- err:
		default:
		}
		close(p.waitCh)
	})
}

type startCall struct {
	sourceURL string
	pushURL   string
}

type fakeRunner struct {
	mu       sync.Mutex
	calls    []startCall
	ctxs     []context.Context
	queue    []*fakeProcess
	startErr error
}

func (r *fakeRunner) Start(ctx context.Context, sourceURL, pushURL string) (Process, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, startCall{sourceURL: sourceURL, pushURL: pushURL})
	r.ctxs = append(r.ctxs, ctx)
	if r.startErr != nil {
		return nil, r.startErr
	}
	if len(r.queue) == 0 {
		p := newFakeProcess()
		r.queue = append(r.queue, p)
	}
	p := r.queue[0]
	r.queue = r.queue[1:]
	return p, nil
}

func (r *fakeRunner) enqueue(ps ...*fakeProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queue = append(r.queue, ps...)
}

func (r *fakeRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *fakeRunner) lastCtx() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.ctxs) == 0 {
		return nil
	}
	return r.ctxs[len(r.ctxs)-1]
}

func (r *fakeRunner) pushURLs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	for i, c := range r.calls {
		out[i] = c.pushURL
	}
	return out
}

type fakeLiveIDs struct {
	dests map[uuid.UUID][]Destination
	err   error
}

func (f *fakeLiveIDs) ListActiveForClient(_ context.Context, clientID uuid.UUID) ([]Destination, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.dests[clientID], nil
}

type fakeLive struct {
	live bool
}

func (f *fakeLive) SomethingLive() bool { return f.live }

func testDests(client uuid.UUID, n int) []Destination {
	out := make([]Destination, n)
	for i := 0; i < n; i++ {
		out[i] = Destination{
			LiveID:       uuid.New(),
			PlatformName: "plat" + string(rune('A'+i)),
			PushURL:      "rtmp://example/" + string(rune('a'+i)),
		}
	}
	return out
}

func newTestManager(t *testing.T, client uuid.UUID, dests []Destination, live bool) (*Manager, *fakeRunner, *obsmock.Mock, []*fakeProcess) {
	t.Helper()
	obsCtl := obsmock.New()
	runner := &fakeRunner{}
	procs := make([]*fakeProcess, len(dests))
	for i := range dests {
		procs[i] = newFakeProcess()
	}
	runner.enqueue(procs...)
	provider := &fakeLiveIDs{dests: map[uuid.UUID][]Destination{client: dests}}
	m := NewManager(obsCtl, provider, "rtmp://localhost:1935/program", runner, nil, &fakeLive{live: live})
	return m, runner, obsCtl, procs
}

// UT-054
func TestSetActiveClient_WhileStopped(t *testing.T) {
	clientA := uuid.New()
	m, _, _, _ := newTestManager(t, clientA, nil, true)
	if err := m.SetActiveClient(context.Background(), clientA); err != nil {
		t.Fatalf("SetActiveClient: %v", err)
	}
	got := m.ActiveClient()
	if got == nil || *got != clientA {
		t.Fatalf("ActiveClient = %v, want %v", got, clientA)
	}
}

// UT-055
func TestSetActiveClient_WhileBroadcasting(t *testing.T) {
	clientA, clientB := uuid.New(), uuid.New()
	dests := testDests(clientA, 1)
	m, _, _, _ := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	err := m.SetActiveClient(context.Background(), clientB)
	if !errors.Is(err, ErrClientSwitchWhileBroadcasting) {
		t.Fatalf("got %v, want ErrClientSwitchWhileBroadcasting", err)
	}
}

// UT-056
func TestSetActiveClient_SameClientIdempotent(t *testing.T) {
	clientA := uuid.New()
	m, _, _, _ := newTestManager(t, clientA, nil, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	if err := m.SetActiveClient(context.Background(), clientA); err != nil {
		t.Fatalf("second SetActiveClient: %v", err)
	}
}

// UT-057
func TestStart_TwoDestinations(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 2)
	m, runner, obsCtl, _ := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if runner.callCount() != 2 {
		t.Fatalf("runner starts = %d, want 2", runner.callCount())
	}
	urls := runner.pushURLs()
	want := map[string]bool{dests[0].PushURL: true, dests[1].PushURL: true}
	for _, u := range urls {
		if !want[u] {
			t.Fatalf("unexpected push URL %q", u)
		}
	}
	if obsCtl.StartProgramStreamCalls != 1 {
		t.Fatalf("StartProgramStreamCalls = %d", obsCtl.StartProgramStreamCalls)
	}
	if obsCtl.LastStreamURL != "rtmp://localhost:1935/program" {
		t.Fatalf("LastStreamURL = %q", obsCtl.LastStreamURL)
	}
}

// UT-058
func TestStart_NoActiveClient(t *testing.T) {
	m, runner, _, _ := newTestManager(t, uuid.New(), nil, true)
	err := m.Start(context.Background())
	if !errors.Is(err, ErrNoActiveClient) {
		t.Fatalf("got %v", err)
	}
	if runner.callCount() != 0 {
		t.Fatalf("unexpected starts")
	}
}

// UT-059
func TestStart_NoDestinations(t *testing.T) {
	clientA := uuid.New()
	m, runner, _, _ := newTestManager(t, clientA, nil, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	err := m.Start(context.Background())
	if !errors.Is(err, ErrNoDestinationsConfigured) {
		t.Fatalf("got %v", err)
	}
	if runner.callCount() != 0 {
		t.Fatalf("unexpected starts")
	}
}

// UT-060
func TestStart_NothingLive(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 1)
	m, runner, _, _ := newTestManager(t, clientA, dests, false)
	_ = m.SetActiveClient(context.Background(), clientA)
	err := m.Start(context.Background())
	if !errors.Is(err, ErrNothingLive) {
		t.Fatalf("got %v", err)
	}
	if runner.callCount() != 0 {
		t.Fatalf("unexpected starts")
	}
}

// UT-061
func TestStart_Idempotent(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 2)
	m, runner, _, _ := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if runner.callCount() != 2 {
		t.Fatalf("starts = %d, want 2", runner.callCount())
	}
}

// UT-062
func TestStop_KillsAllAndStopsStream(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 2)
	m, _, obsCtl, procs := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	for i, p := range procs {
		p.mu.Lock()
		killed := p.killed
		p.mu.Unlock()
		if !killed {
			t.Fatalf("proc %d not killed", i)
		}
	}
	if obsCtl.StopProgramStreamCalls != 1 {
		t.Fatalf("StopProgramStreamCalls = %d", obsCtl.StopProgramStreamCalls)
	}
	for _, st := range m.Status() {
		if st.State != StateStopped {
			t.Fatalf("expected stopped, got %+v", st)
		}
	}
}

// UT-063
func TestStop_AlreadyStopped(t *testing.T) {
	clientA := uuid.New()
	m, _, _, _ := newTestManager(t, clientA, nil, true)
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// UT-064
func TestStatus_ConnectedWhileRunning(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 1)
	m, _, _, _ := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	st := m.Status()
	if len(st) != 1 || st[0].State != StateConnected {
		t.Fatalf("status = %+v", st)
	}
}

// UT-065
func TestStatus_IsolatedFailure(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 2)
	m, _, _, procs := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	procs[0].exit(errors.New("boom"))
	waitFor(t, func() bool {
		for _, st := range m.Status() {
			if st.LiveID == dests[0].LiveID && st.State == StateFailed {
				return true
			}
		}
		return false
	})
	byID := map[uuid.UUID]DestinationStatus{}
	for _, st := range m.Status() {
		byID[st.LiveID] = st
	}
	if byID[dests[0].LiveID].State != StateFailed {
		t.Fatalf("dest0: %+v", byID[dests[0].LiveID])
	}
	if byID[dests[1].LiveID].State != StateConnected {
		t.Fatalf("dest1: %+v", byID[dests[1].LiveID])
	}
	if !m.Running() {
		t.Fatalf("manager should stay running")
	}
}

// UT-066
func TestRestartDestination_Failed(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 2)
	m, runner, _, procs := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	procs[0].exit(errors.New("fail"))
	waitFor(t, func() bool {
		for _, st := range m.Status() {
			if st.LiveID == dests[0].LiveID && st.State == StateFailed {
				return true
			}
		}
		return false
	})
	newProc := newFakeProcess()
	runner.enqueue(newProc)
	before := runner.callCount()
	if err := m.RestartDestination(context.Background(), dests[0].LiveID); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if runner.callCount() != before+1 {
		t.Fatalf("expected one new start")
	}
	// other dest still connected
	for _, st := range m.Status() {
		if st.LiveID == dests[1].LiveID && st.State != StateConnected {
			t.Fatalf("other dest disturbed: %+v", st)
		}
		if st.LiveID == dests[0].LiveID && st.State != StateConnected {
			t.Fatalf("restarted not connected: %+v", st)
		}
	}
}

// UT-067
func TestRestartDestination_NotFound(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 1)
	m, _, _, _ := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	err := m.RestartDestination(context.Background(), uuid.New())
	if !errors.Is(err, ErrDestinationNotFound) {
		t.Fatalf("got %v", err)
	}
}

// UT-068
func TestRestartDestination_WhenStopped(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 1)
	m, runner, _, _ := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	_ = m.Stop(context.Background())
	before := runner.callCount()
	err := m.RestartDestination(context.Background(), dests[0].LiveID)
	if !errors.Is(err, ErrBroadcastStopped) {
		t.Fatalf("got %v", err)
	}
	if runner.callCount() != before {
		t.Fatalf("unexpected start")
	}
}

// UT-069
func TestRestartDestination_AlreadyConnected(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 2)
	m, runner, _, _ := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	newProc := newFakeProcess()
	runner.enqueue(newProc)
	before := runner.callCount()
	if err := m.RestartDestination(context.Background(), dests[0].LiveID); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if runner.callCount() != before+1 {
		t.Fatalf("expected reconnect")
	}
	for _, st := range m.Status() {
		if st.LiveID == dests[1].LiveID && st.State != StateConnected {
			t.Fatalf("other disturbed: %+v", st)
		}
	}
}

// UT-070
func TestRestartDestination_FailedBecomesConnected(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 1)
	m, runner, _, procs := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	procs[0].exit(errors.New("gone"))
	waitFor(t, func() bool {
		st := m.Status()
		return len(st) == 1 && st[0].State == StateFailed
	})
	runner.enqueue(newFakeProcess())
	_ = m.RestartDestination(context.Background(), dests[0].LiveID)
	st := m.Status()
	if len(st) != 1 || st[0].State != StateConnected {
		t.Fatalf("status = %+v", st)
	}
}

// UT-071
func TestAllProcessesExit_NoAutoStop(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 2)
	m, _, obsCtl, procs := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)
	_ = m.Start(context.Background())
	procs[0].exit(errors.New("a"))
	procs[1].exit(errors.New("b"))
	waitFor(t, func() bool {
		st := m.Status()
		if len(st) != 2 {
			return false
		}
		for _, s := range st {
			if s.State != StateFailed {
				return false
			}
		}
		return true
	})
	if !m.Running() {
		t.Fatalf("should not auto-stop")
	}
	if obsCtl.StopProgramStreamCalls != 0 {
		t.Fatalf("should not stop OBS stream")
	}
}

// Regression: spawned relay processes must be detached from the caller's
// context (e.g. an HTTP request that returns right after Start), so a real
// exec.CommandContext process is not killed when the request context is
// cancelled. Teardown happens only via Kill on Stop/Restart/shutdown.
func TestStart_ProcessDetachedFromCallerContext(t *testing.T) {
	clientA := uuid.New()
	dests := testDests(clientA, 1)
	m, runner, _, _ := newTestManager(t, clientA, dests, true)
	_ = m.SetActiveClient(context.Background(), clientA)

	ctx, cancel := context.WithCancel(context.Background())
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	procCtx := runner.lastCtx()
	if procCtx == nil {
		t.Fatal("runner never called")
	}
	cancel() // caller's context is done once its work returns
	select {
	case <-procCtx.Done():
		t.Fatal("process context cancelled when caller context was cancelled")
	default:
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met in time")
}

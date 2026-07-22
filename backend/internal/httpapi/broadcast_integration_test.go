package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"live-orchestrator/backend/internal/broadcast"
	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/obs/obsmock"
	"live-orchestrator/backend/internal/orchestrator"
	"live-orchestrator/backend/internal/pagination"
	"live-orchestrator/backend/internal/positions"
	"live-orchestrator/backend/internal/scenes"
)

type itFakeProcess struct {
	mu     sync.Mutex
	waitCh chan error
	once   sync.Once
	killed bool
}

func newITFakeProcess() *itFakeProcess {
	return &itFakeProcess{waitCh: make(chan error, 1)}
}

func (p *itFakeProcess) Wait() error {
	err, ok := <-p.waitCh
	if !ok {
		return nil
	}
	return err
}

func (p *itFakeProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.killed {
		return nil
	}
	p.killed = true
	p.once.Do(func() {
		select {
		case p.waitCh <- nil:
		default:
		}
		close(p.waitCh)
	})
	return nil
}

func (p *itFakeProcess) exit(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.once.Do(func() {
		select {
		case p.waitCh <- err:
		default:
		}
		close(p.waitCh)
	})
}

type itFakeRunner struct {
	mu    sync.Mutex
	queue []*itFakeProcess
	all   []*itFakeProcess
}

func (r *itFakeRunner) Start(_ context.Context, _, _ string) (broadcast.Process, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var p *itFakeProcess
	if len(r.queue) > 0 {
		p = r.queue[0]
		r.queue = r.queue[1:]
	} else {
		p = newITFakeProcess()
	}
	r.all = append(r.all, p)
	return p, nil
}

func (r *itFakeRunner) enqueue(ps ...*itFakeProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queue = append(r.queue, ps...)
}

type itLiveProvider struct {
	mu    sync.Mutex
	dests map[uuid.UUID][]broadcast.Destination
}

func (p *itLiveProvider) ListActiveForClient(_ context.Context, clientID uuid.UUID) ([]broadcast.Destination, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]broadcast.Destination(nil), p.dests[clientID]...), nil
}

func (p *itLiveProvider) set(clientID uuid.UUID, dests []broadcast.Destination) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dests == nil {
		p.dests = map[uuid.UUID][]broadcast.Destination{}
	}
	p.dests[clientID] = dests
}

type broadcastClientSvc struct {
	ok map[uuid.UUID]bool
}

func (c *broadcastClientSvc) Create(context.Context, client.CreateRequest) (*client.Client, error) {
	return nil, errors.New("unused")
}
func (c *broadcastClientSvc) GetByID(_ context.Context, id uuid.UUID) (*client.Client, error) {
	if c.ok[id] {
		return &client.Client{ID: id, Name: "c"}, nil
	}
	return nil, client.ErrNotFound
}
func (c *broadcastClientSvc) List(context.Context, pagination.Request) (pagination.Page[client.Response], error) {
	return pagination.Page[client.Response]{}, errors.New("unused")
}
func (c *broadcastClientSvc) Update(context.Context, uuid.UUID, client.UpdateFields) (*client.Client, error) {
	return nil, errors.New("unused")
}
func (c *broadcastClientSvc) Delete(context.Context, uuid.UUID) error {
	return errors.New("unused")
}

type orchLiveChecker struct {
	orch *orchestrator.Orchestrator
}

func (l *orchLiveChecker) SomethingLive() bool {
	return l.orch.LiveState().LiveKind != orchestrator.LiveKindNone
}

type broadcastFixture struct {
	handler  http.Handler
	orch     *orchestrator.Orchestrator
	media    *fakeMediaClient
	obs      *obsmock.Mock
	bcast    *broadcast.Manager
	runner   *itFakeRunner
	provider *itLiveProvider
	clients  *broadcastClientSvc
}

func newBroadcastFixture(t *testing.T) *broadcastFixture {
	t.Helper()
	dir := t.TempDir()
	store := positions.NewFileStore(filepath.Join(dir, "positions.json"))
	sstore := scenes.NewFileStore(filepath.Join(dir, "scenes.json"))
	media := &fakeMediaClient{}
	obsCtl := obsmock.New()
	hub := events.NewHub()
	orch := orchestrator.New(media, obsCtl, hub, "Program", time.Second, "rtmp://localhost:1935", store, sstore)

	runner := &itFakeRunner{}
	provider := &itLiveProvider{dests: map[uuid.UUID][]broadcast.Destination{}}
	clients := &broadcastClientSvc{ok: map[uuid.UUID]bool{}}
	bcast := broadcast.NewManager(obsCtl, provider, "rtmp://localhost:1935/program", runner, hub, &orchLiveChecker{orch: orch})
	srv := NewServer(orch, hub, testToken, clients, nil, nil, nil, bcast)
	return &broadcastFixture{
		handler:  srv.Handler(),
		orch:     orch,
		media:    media,
		obs:      obsCtl,
		bcast:    bcast,
		runner:   runner,
		provider: provider,
		clients:  clients,
	}
}

func (f *broadcastFixture) registerClient(id uuid.UUID) {
	f.clients.ok[id] = true
}

func (f *broadcastFixture) twoDests(clientID uuid.UUID) []broadcast.Destination {
	d1 := broadcast.Destination{LiveID: uuid.New(), PlatformName: "YT", PushURL: "rtmp://yt/k1"}
	d2 := broadcast.Destination{LiveID: uuid.New(), PlatformName: "TW", PushURL: "rtmp://tw/k2"}
	f.provider.set(clientID, []broadcast.Destination{d1, d2})
	return []broadcast.Destination{d1, d2}
}

func (f *broadcastFixture) goLiveCamera(t *testing.T, cam string) {
	t.Helper()
	onlineCameras(f.media, f.orch, cam)
	if rec := doRequest(f.handler, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"`+cam+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(f.handler, http.MethodPost, "/api/v1/live/cut", ""); rec.Code != http.StatusOK {
		t.Fatalf("cut: %d %s", rec.Code, rec.Body.String())
	}
}

func decodeBroadcast(t *testing.T, rec *httptest.ResponseRecorder) broadcast.StatusSnapshot {
	t.Helper()
	var snap broadcast.StatusSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&snap); err != nil {
		t.Fatalf("decode broadcast: %v body=%s", err, rec.Body.String())
	}
	return snap
}

func waitBroadcast(t *testing.T, f *broadcastFixture, cond func(broadcast.StatusSnapshot) bool) broadcast.StatusSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec := doRequest(f.handler, http.MethodGet, "/api/v1/broadcast", "")
		snap := decodeBroadcast(t, rec)
		if cond(snap) {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("broadcast condition not met")
	return broadcast.StatusSnapshot{}
}

// IT-015
func TestIntegration_Broadcast_SetClient(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rec.Code, rec.Body.String())
	}
	snap := decodeBroadcast(t, doRequest(f.handler, http.MethodGet, "/api/v1/broadcast", ""))
	if snap.ActiveClientID == nil || *snap.ActiveClientID != cid {
		t.Fatalf("activeClientId = %+v", snap.ActiveClientID)
	}
}

// IT-016
func TestIntegration_Broadcast_UnknownClient(t *testing.T) {
	f := newBroadcastFixture(t)
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+uuid.New().String()+`"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// IT-017
func TestIntegration_Broadcast_SwitchWhileRunning(t *testing.T) {
	f := newBroadcastFixture(t)
	a, b := uuid.New(), uuid.New()
	f.registerClient(a)
	f.registerClient(b)
	f.twoDests(a)
	f.goLiveCamera(t, "cam1")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+a.String()+`"}`)
	if rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", ""); rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+b.String()+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

// IT-018
func TestIntegration_Broadcast_StartNoClient(t *testing.T) {
	f := newBroadcastFixture(t)
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

// IT-019
func TestIntegration_Broadcast_StartNothingLive(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	f.twoDests(cid)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d %s", rec.Code, rec.Body.String())
	}
}

// IT-020
func TestIntegration_Broadcast_DoubleStart(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	dests := f.twoDests(cid)
	f.goLiveCamera(t, "cam1")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	r1 := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	r2 := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	if r1.Code != http.StatusOK || r2.Code != http.StatusOK {
		t.Fatalf("codes %d %d", r1.Code, r2.Code)
	}
	snap := decodeBroadcast(t, doRequest(f.handler, http.MethodGet, "/api/v1/broadcast", ""))
	if len(snap.Destinations) != len(dests) {
		t.Fatalf("dest count %d", len(snap.Destinations))
	}
}

// IT-021
func TestIntegration_Broadcast_StartTwoConnected(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	f.twoDests(cid)
	f.goLiveCamera(t, "cam1")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	snap := decodeBroadcast(t, rec)
	if !snap.Running || len(snap.Destinations) != 2 {
		t.Fatalf("snap %+v", snap)
	}
	for _, d := range snap.Destinations {
		if d.State != broadcast.StateConnected {
			t.Fatalf("dest %+v", d)
		}
	}
	if f.obs.StartProgramStreamCalls < 1 {
		t.Fatalf("OBS stream not started")
	}
}

// IT-022
func TestIntegration_Broadcast_CutDoesNotRestart(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	dests := f.twoDests(cid)
	onlineCameras(f.media, f.orch, "cam1", "cam2")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam1"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/live/cut", "")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	before := len(f.runner.all)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/live/preview", `{"kind":"camera","id":"cam2"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/live/cut", "")
	if len(f.runner.all) != before {
		t.Fatalf("processes restarted on cut")
	}
	snap := decodeBroadcast(t, doRequest(f.handler, http.MethodGet, "/api/v1/broadcast", ""))
	if !snap.Running || len(snap.Destinations) != len(dests) {
		t.Fatalf("snap %+v", snap)
	}
}

// IT-023
func TestIntegration_Broadcast_Stop(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	f.twoDests(cid)
	f.goLiveCamera(t, "cam1")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/stop", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: %d", rec.Code)
	}
	snap := decodeBroadcast(t, doRequest(f.handler, http.MethodGet, "/api/v1/broadcast", ""))
	if snap.Running {
		t.Fatalf("still running")
	}
	for _, d := range snap.Destinations {
		if d.State == broadcast.StateConnected {
			t.Fatalf("still connected: %+v", d)
		}
	}
}

// IT-024
func TestIntegration_Broadcast_StopIdempotent(t *testing.T) {
	f := newBroadcastFixture(t)
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/stop", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// IT-025
func TestIntegration_Broadcast_IsolatedFailure(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	dests := f.twoDests(cid)
	p0, p1 := newITFakeProcess(), newITFakeProcess()
	f.runner.enqueue(p0, p1)
	f.goLiveCamera(t, "cam1")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	p0.exit(errors.New("fail"))
	snap := waitBroadcast(t, f, func(s broadcast.StatusSnapshot) bool {
		var failed, connected int
		for _, d := range s.Destinations {
			if d.LiveID == dests[0].LiveID && d.State == broadcast.StateFailed {
				failed++
			}
			if d.LiveID == dests[1].LiveID && d.State == broadcast.StateConnected {
				connected++
			}
		}
		return failed == 1 && connected == 1
	})
	if !snap.Running {
		t.Fatalf("should stay running")
	}
}

// IT-026
func TestIntegration_Broadcast_RestartFailed(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	dests := f.twoDests(cid)
	p0, p1 := newITFakeProcess(), newITFakeProcess()
	f.runner.enqueue(p0, p1)
	f.goLiveCamera(t, "cam1")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	p0.exit(errors.New("fail"))
	waitBroadcast(t, f, func(s broadcast.StatusSnapshot) bool {
		for _, d := range s.Destinations {
			if d.LiveID == dests[0].LiveID && d.State == broadcast.StateFailed {
				return true
			}
		}
		return false
	})
	f.runner.enqueue(newITFakeProcess())
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/destinations/"+dests[0].LiveID.String()+"/restart", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("restart: %d %s", rec.Code, rec.Body.String())
	}
	snap := decodeBroadcast(t, rec)
	for _, d := range snap.Destinations {
		if d.LiveID == dests[0].LiveID && d.State != broadcast.StateConnected {
			t.Fatalf("not reconnected: %+v", d)
		}
	}
}

// IT-027
func TestIntegration_Broadcast_RestartUnknown(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	f.twoDests(cid)
	f.goLiveCamera(t, "cam1")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/destinations/"+uuid.New().String()+"/restart", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// IT-028
func TestIntegration_Broadcast_RestartWhileStopped(t *testing.T) {
	f := newBroadcastFixture(t)
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/destinations/"+uuid.New().String()+"/restart", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

// IT-033
func TestIntegration_WS_BroadcastStatus(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	dests := f.twoDests(cid)
	p0, p1 := newITFakeProcess(), newITFakeProcess()
	f.runner.enqueue(p0, p1)
	f.goLiveCamera(t, "cam1")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", "")

	ts := httptest.NewServer(f.handler)
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Api-Token": []string{testToken}},
	})
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.CloseNow()
	for range 6 {
		var env wsEnvelope
		_ = wsjson.Read(ctx, conn, &env)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		p0.exit(errors.New("ws-fail"))
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		readCtx, c := context.WithTimeout(ctx, 500*time.Millisecond)
		var env wsEnvelope
		err := wsjson.Read(readCtx, conn, &env)
		c()
		if err != nil {
			continue
		}
		if env.Type != "broadcast.status" {
			continue
		}
		raw, _ := json.Marshal(env.Payload)
		var snap broadcast.StatusSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			continue
		}
		for _, d := range snap.Destinations {
			if d.LiveID == dests[0].LiveID && d.State == broadcast.StateFailed {
				return
			}
		}
	}
	t.Fatalf("timed out waiting for broadcast.status failure")
}

// IT-034
func TestIntegration_AuthGuard_NewRoutes(t *testing.T) {
	f := newBroadcastFixture(t)
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/scenes"},
		{http.MethodPost, "/api/v1/scenes"},
		{http.MethodGet, "/api/v1/live"},
		{http.MethodPost, "/api/v1/live/preview"},
		{http.MethodPost, "/api/v1/live/cut"},
		{http.MethodGet, "/api/v1/broadcast"},
		{http.MethodPost, "/api/v1/broadcast/client"},
		{http.MethodPost, "/api/v1/broadcast/start"},
		{http.MethodPost, "/api/v1/broadcast/stop"},
		{http.MethodPost, "/api/v1/broadcast/destinations/" + uuid.New().String() + "/restart"},
	}
	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected 401, got %d", rt.method, rt.path, rec.Code)
		}
	}
}

// E2E-003
func TestE2E_003_MultiPlatformFailureRecovery(t *testing.T) {
	f := newBroadcastFixture(t)
	a, b := uuid.New(), uuid.New()
	f.registerClient(a)
	f.registerClient(b)
	dests := f.twoDests(a)
	p0, p1 := newITFakeProcess(), newITFakeProcess()
	f.runner.enqueue(p0, p1)
	f.goLiveCamera(t, "cam1")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+a.String()+`"}`)
	if rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", ""); rec.Code != http.StatusOK {
		t.Fatalf("start: %d", rec.Code)
	}
	p0.exit(errors.New("down"))
	waitBroadcast(t, f, func(s broadcast.StatusSnapshot) bool {
		var failed, connected int
		for _, d := range s.Destinations {
			if d.State == broadcast.StateFailed {
				failed++
			}
			if d.State == broadcast.StateConnected {
				connected++
			}
		}
		return failed == 1 && connected == 1
	})
	f.runner.enqueue(newITFakeProcess())
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/destinations/"+dests[0].LiveID.String()+"/restart", "")
	snap := decodeBroadcast(t, doRequest(f.handler, http.MethodGet, "/api/v1/broadcast", ""))
	for _, d := range snap.Destinations {
		if d.State != broadcast.StateConnected {
			t.Fatalf("expected all connected: %+v", d)
		}
	}
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+b.String()+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 switch, got %d", rec.Code)
	}
	snap = decodeBroadcast(t, doRequest(f.handler, http.MethodGet, "/api/v1/broadcast", ""))
	if snap.ActiveClientID == nil || *snap.ActiveClientID != a {
		t.Fatalf("client changed: %+v", snap.ActiveClientID)
	}
}

// E2E-005
func TestE2E_005_FullBroadcastCycleWithCut(t *testing.T) {
	f := newBroadcastFixture(t)
	cid := uuid.New()
	f.registerClient(cid)
	f.twoDests(cid)
	onlineCameras(f.media, f.orch, "cam1", "cam2")
	p1 := createPositionHTTP(t, f.handler, "A")
	p2 := createPositionHTTP(t, f.handler, "B")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/positions/"+p1.ID+"/camera", `{"cameraId":"cam1"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/positions/"+p2.ID+"/camera", `{"cameraId":"cam2"}`)
	rec := doRequest(f.handler, http.MethodPost, "/api/v1/scenes", `{"name":"S1","positionIds":["`+p1.ID+`"]}`)
	sc1 := decodeScene(t, rec)
	rec = doRequest(f.handler, http.MethodPost, "/api/v1/scenes", `{"name":"S2","positionIds":["`+p2.ID+`"]}`)
	sc2 := decodeScene(t, rec)

	_ = doRequest(f.handler, http.MethodPost, "/api/v1/live/preview", `{"kind":"scene","id":"`+sc1.ID+`"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/live/cut", "")
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/client", `{"clientId":"`+cid.String()+`"}`)
	if rec := doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/start", ""); rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	before := len(f.runner.all)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/live/preview", `{"kind":"scene","id":"`+sc2.ID+`"}`)
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/live/cut", "")
	if len(f.runner.all) != before {
		t.Fatalf("cut restarted destinations")
	}
	snap := decodeBroadcast(t, doRequest(f.handler, http.MethodGet, "/api/v1/broadcast", ""))
	if !snap.Running {
		t.Fatalf("should still run after cut")
	}
	_ = doRequest(f.handler, http.MethodPost, "/api/v1/broadcast/stop", "")
	snap = decodeBroadcast(t, doRequest(f.handler, http.MethodGet, "/api/v1/broadcast", ""))
	if snap.Running {
		t.Fatalf("expected stopped")
	}
}

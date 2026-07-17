package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/mediaserver"
	"live-orchestrator/backend/internal/obs/obsmock"
)

// fakeMediaClient lets tests script the sequence of ListActiveStreams results.
type fakeMediaClient struct {
	mu      sync.Mutex
	streams []mediaserver.StreamInfo
	err     error
}

func (f *fakeMediaClient) set(streams []mediaserver.StreamInfo, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streams = streams
	f.err = err
}

func (f *fakeMediaClient) ListActiveStreams(ctx context.Context) ([]mediaserver.StreamInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make([]mediaserver.StreamInfo, len(f.streams))
	copy(out, f.streams)
	return out, nil
}

func newTestOrchestrator() (*Orchestrator, *fakeMediaClient, *obsmock.Mock) {
	media := &fakeMediaClient{}
	obsCtl := obsmock.New()
	hub := events.NewHub()
	o := New(media, obsCtl, hub, "Program", time.Second)
	return o, media, obsCtl
}

func TestSyncOnce_CameraAppears(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}}, nil)

	cams := o.SyncOnce(context.Background())

	if len(cams) != 1 {
		t.Fatalf("expected 1 camera, got %d", len(cams))
	}
	if cams[0].ID != "camera1" || cams[0].Status != StatusOnline {
		t.Fatalf("unexpected camera state: %+v", cams[0])
	}
	if !cams[0].ObsSourceCreated {
		t.Fatalf("expected ObsSourceCreated to be true after sync")
	}
	if _, ok := obsCtl.Inputs["cam_camera1"]; !ok {
		t.Fatalf("expected obs input cam_camera1 to be created")
	}
}

func TestSyncOnce_CameraDisappearsAndReturnsBefore60s(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}}, nil)
	o.SyncOnce(context.Background())

	// Camera vanishes.
	media.set(nil, nil)
	cams := o.SyncOnce(context.Background())
	if cams[0].Status != StatusOffline {
		t.Fatalf("expected camera to be offline, got %+v", cams[0])
	}

	// Comes back before the 60s removal window.
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}}, nil)
	cams = o.SyncOnce(context.Background())
	if len(cams) != 1 || cams[0].Status != StatusOnline {
		t.Fatalf("expected camera back online, got %+v", cams)
	}
	if _, ok := obsCtl.Inputs["cam_camera1"]; !ok {
		t.Fatalf("expected obs input to still exist (not removed)")
	}
}

func TestSyncOnce_CameraOffline60sPlusIsRemoved(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}}, nil)
	o.SyncOnce(context.Background())

	// Force the offline timer into the past to simulate 60s+ elapsed.
	media.set(nil, nil)
	o.SyncOnce(context.Background())

	o.mu.Lock()
	o.offlineSince["camera1"] = time.Now().Add(-2 * time.Minute)
	o.mu.Unlock()

	cams := o.SyncOnce(context.Background())
	if len(cams) != 0 {
		t.Fatalf("expected camera to be removed, got %+v", cams)
	}
	if _, ok := obsCtl.Inputs["cam_camera1"]; ok {
		t.Fatalf("expected obs input to be removed")
	}
}

func TestSetLive_OfflineCameraReturnsError(t *testing.T) {
	o, media, _ := newTestOrchestrator()
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}}, nil)
	o.SyncOnce(context.Background())

	// Take it offline.
	media.set(nil, nil)
	o.SyncOnce(context.Background())

	_, err := o.SetLive("camera1")
	if !errors.Is(err, ErrCameraOffline) {
		t.Fatalf("expected ErrCameraOffline, got %v", err)
	}
}

func TestSetLive_UnknownCameraReturnsNotFound(t *testing.T) {
	o, _, _ := newTestOrchestrator()
	_, err := o.SetLive("does-not-exist")
	if !errors.Is(err, ErrCameraNotFound) {
		t.Fatalf("expected ErrCameraNotFound, got %v", err)
	}
}

func TestSetLive_TwoConsecutiveSwitches(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	media.set([]mediaserver.StreamInfo{
		{Name: "camera1", Ready: true},
		{Name: "camera2", Ready: true},
	}, nil)
	o.SyncOnce(context.Background())

	status, err := o.SetLive("camera1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.LiveCameraID != "camera1" {
		t.Fatalf("expected camera1 live, got %q", status.LiveCameraID)
	}
	if obsCtl.VisibleInput["Program"] != "cam_camera1" {
		t.Fatalf("expected cam_camera1 visible in obs, got %q", obsCtl.VisibleInput["Program"])
	}

	status, err = o.SetLive("camera2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.LiveCameraID != "camera2" {
		t.Fatalf("expected camera2 live, got %q", status.LiveCameraID)
	}
	if obsCtl.VisibleInput["Program"] != "cam_camera2" {
		t.Fatalf("expected cam_camera2 visible in obs, got %q", obsCtl.VisibleInput["Program"])
	}

	cams := o.Cameras()
	liveCount := 0
	for _, c := range cams {
		if c.IsLive {
			liveCount++
			if c.ID != "camera2" {
				t.Fatalf("expected camera2 to be the only live camera, found %q live", c.ID)
			}
		}
	}
	if liveCount != 1 {
		t.Fatalf("expected exactly 1 live camera, got %d", liveCount)
	}
}

func TestSetLive_ObsUnreachable(t *testing.T) {
	o, media, obsCtl := newTestOrchestrator()
	media.set([]mediaserver.StreamInfo{{Name: "camera1", Ready: true}}, nil)
	o.SyncOnce(context.Background())

	obsCtl.Connected = false

	_, err := o.SetLive("camera1")
	if !errors.Is(err, ErrOBSUnreachable) {
		t.Fatalf("expected ErrOBSUnreachable, got %v", err)
	}
}

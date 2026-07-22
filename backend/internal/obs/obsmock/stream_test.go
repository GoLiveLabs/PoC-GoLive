package obsmock_test

import (
	"errors"
	"testing"

	"live-orchestrator/backend/internal/obs/obsmock"
)

// UT-050
func TestStartProgramStream_RecordsURLAndStarts(t *testing.T) {
	m := obsmock.New()
	url := "rtmp://mediamtx:1935/program"
	if err := m.StartProgramStream(url); err != nil {
		t.Fatalf("StartProgramStream: %v", err)
	}
	if m.LastStreamURL != url {
		t.Fatalf("expected LastStreamURL %q, got %q", url, m.LastStreamURL)
	}
	if m.StartProgramStreamCalls != 1 {
		t.Fatalf("expected 1 StartProgramStream call, got %d", m.StartProgramStreamCalls)
	}
	if !m.IsStreaming() {
		t.Fatalf("expected IsStreaming true after start")
	}
}

// UT-051
func TestStopProgramStream_Stops(t *testing.T) {
	m := obsmock.New()
	_ = m.StartProgramStream("rtmp://mediamtx:1935/program")
	if err := m.StopProgramStream(); err != nil {
		t.Fatalf("StopProgramStream: %v", err)
	}
	if m.StopProgramStreamCalls != 1 {
		t.Fatalf("expected 1 StopProgramStream call, got %d", m.StopProgramStreamCalls)
	}
	if m.IsStreaming() {
		t.Fatalf("expected IsStreaming false after stop")
	}
}

// UT-052
func TestStartProgramStream_Unreachable_ReturnsError(t *testing.T) {
	m := obsmock.New()
	m.StartProgramStreamErr = errors.New("obs unreachable")
	err := m.StartProgramStream("rtmp://mediamtx:1935/program")
	if err == nil {
		t.Fatalf("expected error when OBS unreachable")
	}
	if m.IsStreaming() {
		t.Fatalf("IsStreaming must stay false after failed start")
	}
}

// UT-053
func TestIsStreaming_StateMachine(t *testing.T) {
	m := obsmock.New()
	if m.IsStreaming() {
		t.Fatalf("expected false before start")
	}
	if err := m.StartProgramStream("rtmp://mediamtx:1935/program"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !m.IsStreaming() {
		t.Fatalf("expected true after start")
	}
	if err := m.StopProgramStream(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if m.IsStreaming() {
		t.Fatalf("expected false after stop")
	}
}

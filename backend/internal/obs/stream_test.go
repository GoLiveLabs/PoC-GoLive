package obs

import "testing"

// UT-052 (real ObsController path): StartProgramStream while disconnected
// returns a wrapped error and IsStreaming stays false.
func TestStartProgramStream_NotConnected_ReturnsError(t *testing.T) {
	o := &ObsController{stopCh: make(chan struct{})}
	err := o.StartProgramStream("rtmp://mediamtx:1935/program")
	if err == nil {
		t.Fatalf("expected error when OBS is not connected")
	}
	if o.IsStreaming() {
		t.Fatalf("IsStreaming must be false when disconnected")
	}
}

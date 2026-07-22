package broadcast

import (
	"context"
	"os/exec"
)

// ProcessRunner is the sole I/O boundary for subprocess management.
type ProcessRunner interface {
	Start(ctx context.Context, sourceURL, pushURL string) (Process, error)
}

// Process is a running ffmpeg (or fake) push process.
type Process interface {
	Wait() error
	Kill() error
}

// ExecRunner starts real ffmpeg processes using FFMPEG_BIN.
type ExecRunner struct {
	Bin string
}

// Start runs: ffmpeg -i sourceURL -c copy -f flv pushURL
func (r *ExecRunner) Start(ctx context.Context, sourceURL, pushURL string) (Process, error) {
	bin := r.Bin
	if bin == "" {
		bin = "ffmpeg"
	}
	cmd := exec.CommandContext(ctx, bin, "-i", sourceURL, "-c", "copy", "-f", "flv", pushURL)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd: cmd}, nil
}

type execProcess struct {
	cmd *exec.Cmd
}

func (p *execProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *execProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

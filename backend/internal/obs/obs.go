// Package obs wraps the obs-websocket v5 client (goobs) behind a small
// interface, so the orchestrator can be tested against a mock.
package obs

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/config"
	"github.com/andreykaipov/goobs/api/requests/general"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/andreykaipov/goobs/api/requests/stream"
	"github.com/andreykaipov/goobs/api/typedefs"
)

// InputKind is the OBS input kind used for camera sources fed by MediaMTX.
const InputKind = "ffmpeg_source"

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
	healthInterval = 5 * time.Second
)

// ErrInputNotFound is returned by position-scoped calls that require an
// existing OBS input (created by CreatePositionInput) and find none.
var ErrInputNotFound = errors.New("obs input not found")

// Controller is the OBS operations the orchestrator depends on. Every
// position-scoped method operates on the single input belonging to a
// position (named by the caller), never on any other scene item.
type Controller interface {
	EnsureScene(name string) error
	// CreatePositionInput creates a new, initially disabled input named
	// inputName in sceneName. It fails if an input with that name already
	// exists — it never updates one.
	CreatePositionInput(sceneName, inputName string) error
	// UpdatePositionSource points inputName's source at url. It fails with
	// ErrInputNotFound if the input doesn't exist — it never creates one.
	UpdatePositionSource(inputName, url string) error
	// SetPositionEnabled toggles inputName's own scene item in sceneName,
	// never touching any other scene item.
	SetPositionEnabled(sceneName, inputName string, enabled bool) error
	// SetInputAudioMuted mutes or unmutes inputName in the program mix.
	SetInputAudioMuted(inputName string, muted bool) error
	RemoveInput(inputName string) error
	IsConnected() bool
	Reconnect() error
	// StartProgramStream configures OBS custom RTMP output to rtmpURL and
	// starts the program stream.
	StartProgramStream(rtmpURL string) error
	// StopProgramStream stops the OBS program stream if it is active.
	StopProgramStream() error
	// IsStreaming reports whether OBS currently has an active stream output.
	IsStreaming() bool
}

// ObsController is the real Controller implementation, backed by goobs.
type ObsController struct {
	addr     string
	password string

	mu        sync.RWMutex
	client    *goobs.Client
	connected bool

	stopOnce sync.Once
	stopCh   chan struct{}
}

// New creates an ObsController and starts a background goroutine that keeps
// the connection alive, reconnecting with exponential backoff on failure.
// The initial connection attempt is best-effort: if OBS is not reachable yet,
// the controller starts disconnected and the background loop keeps retrying.
func New(addr, password string) *ObsController {
	o := &ObsController{
		addr:     addr,
		password: password,
		stopCh:   make(chan struct{}),
	}
	if err := o.connectLocked(); err != nil {
		slog.Warn("initial obs connection failed, will keep retrying", "error", err)
	}
	go o.watchLoop()
	return o
}

// Close stops the background reconnection loop and disconnects from OBS.
func (o *ObsController) Close() {
	o.stopOnce.Do(func() { close(o.stopCh) })
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.client != nil {
		o.client.Disconnect()
	}
}

func (o *ObsController) connectLocked() error {
	client, err := goobs.New(o.addr, goobs.WithPassword(o.password))
	if err != nil {
		o.client = nil
		o.connected = false
		return fmt.Errorf("connecting to obs at %s: %w", o.addr, err)
	}
	o.client = client
	o.connected = true
	return nil
}

// Reconnect tears down any existing connection and dials OBS again.
func (o *ObsController) Reconnect() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.client != nil {
		o.client.Disconnect()
	}
	return o.connectLocked()
}

// IsConnected reports whether the last known connection state is healthy.
func (o *ObsController) IsConnected() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.connected
}

func (o *ObsController) watchLoop() {
	backoff := initialBackoff
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopCh:
			return
		case <-ticker.C:
		}

		if o.healthy() {
			backoff = initialBackoff
			continue
		}

		o.mu.Lock()
		o.connected = false
		o.mu.Unlock()

		slog.Warn("obs connection lost, retrying", "backoff", backoff)
		select {
		case <-o.stopCh:
			return
		case <-time.After(backoff):
		}

		if err := o.Reconnect(); err != nil {
			slog.Warn("obs reconnect attempt failed", "error", err)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		slog.Info("obs reconnected")
		backoff = initialBackoff
	}
}

func (o *ObsController) healthy() bool {
	o.mu.RLock()
	client := o.client
	wasConnected := o.connected
	o.mu.RUnlock()

	if client == nil {
		return false
	}
	if _, err := client.General.GetVersion(&general.GetVersionParams{}); err != nil {
		return false
	}
	if !wasConnected {
		o.mu.Lock()
		o.connected = true
		o.mu.Unlock()
	}
	return true
}

func (o *ObsController) getClient() (*goobs.Client, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.client == nil || !o.connected {
		return nil, fmt.Errorf("obs is not connected")
	}
	return o.client, nil
}

// EnsureScene creates the scene if it doesn't already exist. It is
// idempotent: calling it again for an existing scene is a no-op.
func (o *ObsController) EnsureScene(name string) error {
	client, err := o.getClient()
	if err != nil {
		return err
	}

	list, err := client.Scenes.GetSceneList()
	if err != nil {
		return fmt.Errorf("listing scenes: %w", err)
	}
	for _, s := range list.Scenes {
		if s.SceneName == name {
			return nil
		}
	}

	if _, err := client.Scenes.CreateScene(scenes.NewCreateSceneParams().WithSceneName(name)); err != nil {
		return fmt.Errorf("creating scene %q: %w", name, err)
	}
	return nil
}

// inputSettings builds the ffmpeg_source settings for a camera fed by
// MediaMTX. Field names follow OBS's documented ffmpeg_source properties;
// see DECISIONS.md for why they weren't confirmed against a live
// GetInputDefaultSettings call during implementation.
func inputSettings(url string) map[string]any {
	return map[string]any{
		"input":               url,
		"is_local_file":       false,
		"reconnect_delay_sec": 2,
		"buffering_mb":        1,
	}
}

func (o *ObsController) findInput(inputName string) (bool, error) {
	client, err := o.getClient()
	if err != nil {
		return false, err
	}
	list, err := client.Inputs.GetInputList()
	if err != nil {
		return false, fmt.Errorf("listing inputs: %w", err)
	}
	for _, in := range list.Inputs {
		if in.InputName == inputName {
			return true, nil
		}
	}
	return false, nil
}

// CreatePositionInput creates a new, initially disabled ffmpeg_source input
// named inputName in sceneName, with no source URL configured yet. It fails
// if an input with that name already exists — it never recreates or updates
// one (ADR-003).
func (o *ObsController) CreatePositionInput(sceneName, inputName string) error {
	client, err := o.getClient()
	if err != nil {
		return err
	}

	exists, err := o.findInput(inputName)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("input %q already exists", inputName)
	}

	_, err = client.Inputs.CreateInput(
		inputs.NewCreateInputParams().
			WithSceneName(sceneName).
			WithInputName(inputName).
			WithInputKind(InputKind).
			WithInputSettings(inputSettings("")).
			WithSceneItemEnabled(false),
	)
	if err != nil {
		return fmt.Errorf("creating input %q: %w", inputName, err)
	}
	return nil
}

// UpdatePositionSource points inputName's source at url. It fails with
// ErrInputNotFound if the input doesn't exist — it never creates one
// (ADR-003).
func (o *ObsController) UpdatePositionSource(inputName, url string) error {
	client, err := o.getClient()
	if err != nil {
		return err
	}

	exists, err := o.findInput(inputName)
	if err != nil {
		return err
	}
	if !exists {
		return ErrInputNotFound
	}

	_, err = client.Inputs.SetInputSettings(
		inputs.NewSetInputSettingsParams().
			WithInputName(inputName).
			WithInputSettings(inputSettings(url)).
			WithOverlay(false),
	)
	if err != nil {
		return fmt.Errorf("updating input %q settings: %w", inputName, err)
	}
	return nil
}

// RemoveInput removes an input by name. Removing it also removes all
// associated scene items.
func (o *ObsController) RemoveInput(inputName string) error {
	client, err := o.getClient()
	if err != nil {
		return err
	}
	if _, err := client.Inputs.RemoveInput(inputs.NewRemoveInputParams().WithInputName(inputName)); err != nil {
		return fmt.Errorf("removing input %q: %w", inputName, err)
	}
	return nil
}

// SetPositionEnabled toggles inputName's own scene item in sceneName,
// leaving every other scene item untouched. It fails with ErrInputNotFound
// if the position's input has no scene item in sceneName.
func (o *ObsController) SetPositionEnabled(sceneName, inputName string, enabled bool) error {
	client, err := o.getClient()
	if err != nil {
		return err
	}

	list, err := client.SceneItems.GetSceneItemList(
		sceneitems.NewGetSceneItemListParams().WithSceneName(sceneName),
	)
	if err != nil {
		return fmt.Errorf("listing scene items of %q: %w", sceneName, err)
	}

	for _, item := range list.SceneItems {
		if item.SourceName != inputName {
			continue
		}
		_, err := client.SceneItems.SetSceneItemEnabled(
			sceneitems.NewSetSceneItemEnabledParams().
				WithSceneName(sceneName).
				WithSceneItemId(item.SceneItemID).
				WithSceneItemEnabled(enabled),
		)
		if err != nil {
			return fmt.Errorf("setting visibility of %q: %w", inputName, err)
		}
		return nil
	}
	return ErrInputNotFound
}

// SetInputAudioMuted mutes or unmutes inputName in the program mix
// (ADR-004).
func (o *ObsController) SetInputAudioMuted(inputName string, muted bool) error {
	client, err := o.getClient()
	if err != nil {
		return err
	}
	_, err = client.Inputs.SetInputMute(
		inputs.NewSetInputMuteParams().WithInputName(inputName).WithInputMuted(muted),
	)
	if err != nil {
		return fmt.Errorf("setting mute for %q: %w", inputName, err)
	}
	return nil
}

// StartProgramStream points OBS at rtmpURL via the custom RTMP service and
// starts the stream output.
func (o *ObsController) StartProgramStream(rtmpURL string) error {
	client, err := o.getClient()
	if err != nil {
		return fmt.Errorf("start program stream: %w", err)
	}
	_, err = client.Config.SetStreamServiceSettings(
		config.NewSetStreamServiceSettingsParams().
			WithStreamServiceType("rtmp_custom").
			WithStreamServiceSettings(&typedefs.StreamServiceSettings{
				Server: rtmpURL,
				Key:    "",
			}),
	)
	if err != nil {
		return fmt.Errorf("setting stream service to %q: %w", rtmpURL, err)
	}
	if _, err := client.Stream.StartStream(&stream.StartStreamParams{}); err != nil {
		return fmt.Errorf("starting stream: %w", err)
	}
	return nil
}

// StopProgramStream stops the OBS stream output.
func (o *ObsController) StopProgramStream() error {
	client, err := o.getClient()
	if err != nil {
		return fmt.Errorf("stop program stream: %w", err)
	}
	if _, err := client.Stream.StopStream(&stream.StopStreamParams{}); err != nil {
		return fmt.Errorf("stopping stream: %w", err)
	}
	return nil
}

// IsStreaming reports whether the OBS stream output is currently active.
// Returns false when OBS is unreachable or the status query fails.
func (o *ObsController) IsStreaming() bool {
	client, err := o.getClient()
	if err != nil {
		return false
	}
	status, err := client.Stream.GetStreamStatus()
	if err != nil {
		return false
	}
	return status.OutputActive
}

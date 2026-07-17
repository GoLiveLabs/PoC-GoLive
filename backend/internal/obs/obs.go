// Package obs wraps the obs-websocket v5 client (goobs) behind a small
// interface, so the orchestrator can be tested against a mock.
package obs

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/general"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/andreykaipov/goobs/api/requests/scenes"
)

// InputKind is the OBS input kind used for camera sources fed by MediaMTX.
const InputKind = "ffmpeg_source"

// CamPrefix prefixes every input created by the orchestrator, so it never
// collides with sources the operator created manually in OBS.
const CamPrefix = "cam_"

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
	healthInterval = 5 * time.Second
)

// Controller is the OBS operations the orchestrator depends on.
type Controller interface {
	EnsureScene(name string) error
	CreateCameraInput(sceneName, inputName, url string) error
	RemoveInput(inputName string) error
	// SetOnlyVisibleSource shows inputName and hides every other scene item
	// prefixed with CamPrefix in sceneName.
	SetOnlyVisibleSource(sceneName, inputName string) error
	IsConnected() bool
	Reconnect() error
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

// CreateCameraInput creates a ffmpeg_source input for the given URL and adds
// it to sceneName. If the input already exists, its settings are updated
// instead of failing.
func (o *ObsController) CreateCameraInput(sceneName, inputName, url string) error {
	client, err := o.getClient()
	if err != nil {
		return err
	}

	list, err := client.Inputs.GetInputList()
	if err != nil {
		return fmt.Errorf("listing inputs: %w", err)
	}
	for _, in := range list.Inputs {
		if in.InputName == inputName {
			_, err := client.Inputs.SetInputSettings(
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
	}

	_, err = client.Inputs.CreateInput(
		inputs.NewCreateInputParams().
			WithSceneName(sceneName).
			WithInputName(inputName).
			WithInputKind(InputKind).
			WithInputSettings(inputSettings(url)).
			WithSceneItemEnabled(true),
	)
	if err != nil {
		return fmt.Errorf("creating input %q: %w", inputName, err)
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

// SetOnlyVisibleSource enables inputName's scene item in sceneName and
// disables every other scene item prefixed with CamPrefix.
func (o *ObsController) SetOnlyVisibleSource(sceneName, inputName string) error {
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
		if !strings.HasPrefix(item.SourceName, CamPrefix) {
			continue
		}
		enabled := item.SourceName == inputName
		_, err := client.SceneItems.SetSceneItemEnabled(
			sceneitems.NewSetSceneItemEnabledParams().
				WithSceneName(sceneName).
				WithSceneItemId(item.SceneItemID).
				WithSceneItemEnabled(enabled),
		)
		if err != nil {
			return fmt.Errorf("setting visibility of %q: %w", item.SourceName, err)
		}
	}
	return nil
}

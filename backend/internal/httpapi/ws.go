package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const wsHeartbeatInterval = 30 * time.Second

// wsEnvelope is the wire format for every event sent to WebSocket clients.
type wsEnvelope struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:4200", "127.0.0.1:4200"},
	})
	if err != nil {
		slog.Warn("ws accept failed", "error", err)
		return
	}
	defer conn.CloseNow()

	untrack := s.trackConn(conn)
	defer untrack()

	// Clients don't send messages on this connection; CloseRead drains and
	// discards anything they do send, and answers control frames for us.
	ctx := conn.CloseRead(context.Background())

	if err := writeWSEvent(ctx, conn, "cameras.updated", s.orch.Cameras()); err != nil {
		return
	}
	if err := writeWSEvent(ctx, conn, "system.status", s.orch.Status()); err != nil {
		return
	}
	if err := writeWSEvent(ctx, conn, "positions.updated", s.orch.Positions()); err != nil {
		return
	}
	if err := writeWSEvent(ctx, conn, "scenes.updated", s.orch.Scenes()); err != nil {
		return
	}
	if err := writeWSEvent(ctx, conn, "live.updated", s.orch.LiveState()); err != nil {
		return
	}
	if s.broadcast != nil {
		if err := writeWSEvent(ctx, conn, "broadcast.status", s.broadcast.Snapshot()); err != nil {
			return
		}
	}

	ch, cancel := s.hub.Subscribe()
	defer cancel()

	heartbeat := time.NewTicker(wsHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeWSEvent(ctx, conn, ev.Type, ev.Payload); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := conn.Ping(ctx); err != nil {
				return
			}
		}
	}
}

func writeWSEvent(ctx context.Context, conn *websocket.Conn, eventType string, payload any) error {
	return wsjson.Write(ctx, conn, wsEnvelope{Type: eventType, Payload: payload})
}

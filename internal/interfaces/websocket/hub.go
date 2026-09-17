// Package websocket implements the real-time operation/event feed described
// in §28. It uses Server-Sent Events (simpler than a full WebSocket for a
// server-to-client-only stream) so the frontend can show live workflow
// progress ("Provisioning machine 3/13", "Waiting for Argo CD", ...).
package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Hub fans out published events to every currently-connected SSE client.
// It is intentionally best-effort and non-durable: durable workflow state
// lives in Postgres (ADR-0005); this hub only pushes live updates to
// browsers that happen to be connected right now.
type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan []byte]struct{})}
}

// Publish implements ports.EventBus by fanning event out (as JSON) to every
// connected SSE client on channel. The channel argument is accepted for
// interface compatibility but all clients currently share one global stream;
// callers filter client-side by the event's TargetKind/TargetID.
func (h *Hub) Publish(_ context.Context, _ string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- data:
		default: // drop for slow/disconnected clients rather than block publishers
		}
	}
	return nil
}

// ServeSSE handles GET /api/v1/events/stream, keeping the connection open
// and writing each published event as an SSE `data:` frame.
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := make(chan []byte, 32)
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, client)
		h.mu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-client:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

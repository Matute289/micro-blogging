package ws

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	timelineapp "UalaTwitter/internal/timeline/application"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Handler upgrades GET /ws/timeline to a WebSocket connection.
type Handler struct {
	svc *timelineapp.TimelineService
	hub *Hub
}

// NewHandler creates a Handler backed by the given service and hub.
func NewHandler(svc *timelineapp.TimelineService, hub *Hub) *Handler {
	return &Handler{svc: svc, hub: hub}
}

// RegisterRoutes registers GET /ws/timeline on the router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/ws/timeline", h.serve)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "missing user_id", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "err", err)
		return
	}

	c := &client{
		userID: userID,
		conn:   conn,
		send:   make(chan []byte, 64),
		hub:    h.hub,
	}
	h.hub.register(c)
	defer h.hub.unregister(c)

	// Send current timeline immediately after connecting.
	tweets, err := h.svc.GetTimeline(r.Context(), userID, 20, "")
	if err == nil {
		data, _ := json.Marshal(wsMessage{Type: "timeline", Data: tweets})
		c.tryWrite(data)
	}

	go c.writePump()
	c.readPump() // blocks until the client disconnects
}

// readPump reads from the WebSocket connection to handle pong frames and detect disconnects.
func (c *client) readPump() {
	defer c.conn.Close()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

// writePump writes outgoing messages from the send channel to the WebSocket connection.
// It also sends periodic pings to keep the connection alive.
func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel closed by hub.unregister.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

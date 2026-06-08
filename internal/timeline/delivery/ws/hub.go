package ws

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"

	tweetdomain "UalaTwitter/internal/tweet/domain"
)

// wsMessage is the JSON envelope sent to every WebSocket client.
type wsMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// client is a single WebSocket connection for one user.
type client struct {
	userID string
	conn   *websocket.Conn
	send   chan []byte
	hub    *Hub
	mu     sync.Mutex
	closed bool
}

// tryWrite sends data to the client's buffered channel.
// It is a no-op if the client is already closed or the buffer is full.
func (c *client) tryWrite(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

// Hub tracks all active WebSocket clients, keyed by user ID.
type Hub struct {
	mu      sync.RWMutex
	clients map[string][]*client
}

// NewHub returns an empty, ready-to-use Hub.
func NewHub() *Hub {
	return &Hub{clients: make(map[string][]*client)}
}

// NopNotifier satisfies the application.Notifier interface without doing anything.
// Use it in tests that instantiate TimelineService but don't need WS push.
type NopNotifier struct{}

func (NopNotifier) Notify(_ context.Context, _ string, _ *tweetdomain.Tweet) {}

func (h *Hub) register(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c.userID] = append(h.clients[c.userID], c)
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	list := h.clients[c.userID]
	for i, cl := range list {
		if cl == c {
			h.clients[c.userID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(h.clients[c.userID]) == 0 {
		delete(h.clients, c.userID)
	}
	h.mu.Unlock()

	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.send)
	}
	c.mu.Unlock()
}

// Notify implements application.Notifier — pushes the tweet to every connected
// client for the given userID. Clients that are full or closed are silently skipped.
func (h *Hub) Notify(_ context.Context, userID string, tweet *tweetdomain.Tweet) {
	h.mu.RLock()
	list := make([]*client, len(h.clients[userID]))
	copy(list, h.clients[userID])
	h.mu.RUnlock()

	if len(list) == 0 {
		return
	}
	data, err := json.Marshal(wsMessage{Type: "tweet", Data: tweet})
	if err != nil {
		return
	}
	for _, c := range list {
		c.tryWrite(data)
	}
}

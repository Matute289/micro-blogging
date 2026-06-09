package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tweetdomain "UalaTwitter/internal/tweet/domain"
)

func TestHub_NotifyWithNoClients_NoPanic(t *testing.T) {
	hub := NewHub()
	// Must not panic when no clients are registered
	hub.Notify(context.Background(), "user1", &tweetdomain.Tweet{ID: "t1"})
}

func TestHub_NotifyDeliversTweetToRegisteredClient(t *testing.T) {
	hub := NewHub()
	c := &client{
		userID: "user1",
		send:   make(chan []byte, 1),
		hub:    hub,
	}
	hub.register(c)

	tweet := &tweetdomain.Tweet{
		ID:        "tweet-1",
		UserID:    "poster-1",
		Text:      "hello",
		CreatedAt: time.Now(),
	}
	hub.Notify(context.Background(), "user1", tweet)

	select {
	case msg := <-c.send:
		var envelope wsMessage
		if err := json.Unmarshal(msg, &envelope); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if envelope.Type != "tweet" {
			t.Errorf("want type 'tweet', got %q", envelope.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no message received within 1s")
	}
}

func TestHub_NoDeliveryToClosedClient(t *testing.T) {
	hub := NewHub()
	c := &client{
		userID: "user1",
		send:   make(chan []byte, 1),
		hub:    hub,
	}
	hub.register(c)

	// Manually close the client (simulates what unregister does) without removing from hub
	c.mu.Lock()
	c.closed = true
	close(c.send)
	c.mu.Unlock()

	// tryWrite must be a no-op on a closed client — no panic
	hub.Notify(context.Background(), "user1", &tweetdomain.Tweet{ID: "t1"})

	// Channel is closed; verify no message was written (channel would be drained if written)
	select {
	case msg, ok := <-c.send:
		if ok {
			t.Errorf("expected no delivery to closed client, got: %s", msg)
		}
		// ok=false means channel closed — expected
	default:
		// nothing in channel — also expected
	}
}

func TestHub_UnregisterRemovesClient(t *testing.T) {
	hub := NewHub()
	c := &client{userID: "user1", send: make(chan []byte, 1), hub: hub}
	hub.register(c)
	hub.unregister(c)

	hub.mu.RLock()
	_, exists := hub.clients["user1"]
	hub.mu.RUnlock()
	if exists {
		t.Error("client should be removed from hub after unregister")
	}
}

func TestHub_ConcurrentNotifyAndUnregister(t *testing.T) {
	hub := NewHub()
	c := &client{userID: "user1", send: make(chan []byte, 64), hub: hub}
	hub.register(c)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			hub.Notify(context.Background(), "user1", &tweetdomain.Tweet{ID: "t1"})
		}
	}()

	hub.unregister(c)
	<-done // wait for goroutine to finish — no race, no panic
}

func TestHub_MultipleClientsForSameUser(t *testing.T) {
	hub := NewHub()
	c1 := &client{userID: "user1", send: make(chan []byte, 1), hub: hub}
	c2 := &client{userID: "user1", send: make(chan []byte, 1), hub: hub}
	hub.register(c1)
	hub.register(c2)

	hub.Notify(context.Background(), "user1", &tweetdomain.Tweet{ID: "t1", Text: "hi"})

	for i, c := range []*client{c1, c2} {
		select {
		case <-c.send:
		case <-time.After(time.Second):
			t.Errorf("client %d: no message received", i+1)
		}
	}
}

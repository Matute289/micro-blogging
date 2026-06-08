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

func TestHub_NoDeliveryAfterUnregister(t *testing.T) {
	hub := NewHub()
	c := &client{
		userID: "user1",
		send:   make(chan []byte, 1),
		hub:    hub,
	}
	hub.register(c)
	hub.unregister(c) // closes c.send, marks closed

	// Notify after unregister: tryWrite must not panic and must not deliver
	tweet := &tweetdomain.Tweet{ID: "t1"}
	hub.Notify(context.Background(), "user1", tweet)

	// The hub must have removed the client
	hub.mu.RLock()
	_, exists := hub.clients["user1"]
	hub.mu.RUnlock()
	if exists {
		t.Error("client should be removed from hub after unregister")
	}
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

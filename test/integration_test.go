//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	followapp "UalaTwitter/internal/follow/application"
	followhandler "UalaTwitter/internal/follow/delivery/http"
	followpg "UalaTwitter/internal/follow/infrastructure/postgres"
	timelineapp "UalaTwitter/internal/timeline/application"
	timelinews "UalaTwitter/internal/timeline/delivery/ws"
	timelinequeue "UalaTwitter/internal/timeline/infrastructure/queue"
	timelineredis "UalaTwitter/internal/timeline/infrastructure/redis"
	tweetapp "UalaTwitter/internal/tweet/application"
	tweethandler "UalaTwitter/internal/tweet/delivery/http"
	tweetmongo "UalaTwitter/internal/tweet/infrastructure/mongo"
	userapp "UalaTwitter/internal/user/application"
	userhandler "UalaTwitter/internal/user/delivery/http"
	userpg "UalaTwitter/internal/user/infrastructure/postgres"
	appjwt "UalaTwitter/pkg/jwt"
	mw "UalaTwitter/pkg/middleware"
)

const testJWTSecret = "test-secret-for-integration-32ch"

func makeToken(t *testing.T, userID, username string) string {
	t.Helper()
	tok, err := appjwt.Issue(userID, username, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func buildServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pgPool, err := pgxpool.New(ctx, envOr("POSTGRES_URL", "postgres://twitter:twitter@localhost:5432/twitter?sslmode=disable"))
	if err != nil {
		t.Fatalf("postgres connect: %v", err)
	}
	t.Cleanup(pgPool.Close)

	if _, err := pgPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id           UUID PRIMARY KEY,
			username     TEXT UNIQUE NOT NULL,
			created_at   TIMESTAMPTZ NOT NULL,
			last_seen_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE IF NOT EXISTS follows (
			follower_id  UUID NOT NULL REFERENCES users(id),
			following_id UUID NOT NULL REFERENCES users(id),
			created_at   TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (follower_id, following_id)
		);
		CREATE INDEX IF NOT EXISTS idx_follows_follower  ON follows(follower_id);
		CREATE INDEX IF NOT EXISTS idx_follows_following ON follows(following_id);
		ALTER TABLE users ADD COLUMN IF NOT EXISTS oauth_provider TEXT;
		ALTER TABLE users ADD COLUMN IF NOT EXISTS oauth_sub      TEXT;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oauth
		    ON users(oauth_provider, oauth_sub)
		    WHERE oauth_provider IS NOT NULL;
	`); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(envOr("MONGO_URL", "mongodb://localhost:27017")))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	t.Cleanup(func() { _ = mongoClient.Disconnect(context.Background()) })

	mongoDB := mongoClient.Database(envOr("MONGO_DB", "twitter"))

	redisOpts, err := redis.ParseURL(envOr("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		t.Fatalf("redis url: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)
	t.Cleanup(func() { _ = redisClient.Close() })

	userRepo := userpg.NewUserRepository(pgPool)
	followRepo := followpg.NewFollowRepository(pgPool)
	tweetRepo := tweetmongo.NewTweetRepository(mongoDB)
	timelineCache := timelineredis.NewTimelineCache(redisClient)

	wsHub := timelinews.NewHub()
	userSvc := userapp.NewUserService(userRepo)
	followSvc := followapp.NewFollowService(followRepo)
	tweetSvc := tweetapp.NewTweetService(tweetRepo)
	timelineSvc := timelineapp.NewTimelineService(followRepo, tweetRepo, timelineCache, wsHub)

	fanOutQueue := timelinequeue.NewRedisQueue(redisClient)
	go fanOutQueue.RunWorker(ctx, timelineSvc.FanOutTweet)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	userhandler.NewUserHandler(userSvc).RegisterRoutes(r)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(mw.JWT(testJWTSecret))
		followhandler.NewFollowHandler(followSvc).RegisterRoutes(r)
		tweethandler.NewTweetHandler(tweetSvc, fanOutQueue).RegisterRoutes(r)
	})

	// WS uses ?token= JWT (validated inside handler)
	timelinews.NewHandler(timelineSvc, wsHub, testJWTSecret).RegisterRoutes(r)

	return httptest.NewServer(r)
}

type userResp struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type tweetResp struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Text   string `json:"text"`
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

func wsURL(srv *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + path
}

func TestIntegration(t *testing.T) {
	srv := buildServer(t)
	defer srv.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	client := srv.Client()

	do := func(method, path string, body io.Reader, headers map[string]string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+path, body)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		return resp
	}

	bearerAuth := func(id, username string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + makeToken(t, id, username)}
	}

	// Setup: create users.
	var alice, bob userResp
	for _, tc := range []struct {
		name string
		dst  *userResp
	}{
		{"alice_" + suffix, &alice},
		{"bob_" + suffix, &bob},
	} {
		resp := do("POST", "/users", jsonBody(map[string]string{"username": tc.name}), nil)
		if resp.StatusCode != http.StatusCreated {
			resp.Body.Close()
			t.Fatalf("create user %s: got %d", tc.name, resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(tc.dst); err != nil {
			resp.Body.Close()
			t.Fatalf("decode user %s: %v", tc.name, err)
		}
		resp.Body.Close()
	}

	// ---- POST /users ----

	t.Run("POST /users duplicate username returns 409", func(t *testing.T) {
		resp := do("POST", "/users", jsonBody(map[string]string{"username": "alice_" + suffix}), nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("want 409, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /users missing username returns 400", func(t *testing.T) {
		resp := do("POST", "/users", jsonBody(map[string]string{"username": ""}), nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	// ---- GET /users/:id ----

	t.Run("GET /users/:id returns user", func(t *testing.T) {
		resp := do("GET", "/users/"+alice.ID, nil, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}
		var u userResp
		if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if u.ID != alice.ID {
			t.Fatalf("want id %s, got %s", alice.ID, u.ID)
		}
	})

	t.Run("GET /users/:id unknown returns 404", func(t *testing.T) {
		resp := do("GET", "/users/00000000-0000-0000-0000-000000000000", nil, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("want 404, got %d", resp.StatusCode)
		}
	})

	// Setup: bob posts an initial tweet.
	var bobTweet tweetResp
	{
		resp := do("POST", "/tweets", jsonBody(map[string]string{"text": "hello from bob"}), bearerAuth(bob.ID, bob.Username))
		if resp.StatusCode != http.StatusCreated {
			resp.Body.Close()
			t.Fatalf("bob post tweet: got %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&bobTweet); err != nil {
			resp.Body.Close()
			t.Fatalf("decode tweet: %v", err)
		}
		resp.Body.Close()
		if bobTweet.UserID != bob.ID {
			t.Fatalf("tweet user_id: want %s, got %s", bob.ID, bobTweet.UserID)
		}
	}

	// ---- POST /tweets ----

	t.Run("POST /tweets empty text returns 400", func(t *testing.T) {
		resp := do("POST", "/tweets", jsonBody(map[string]string{"text": ""}), bearerAuth(bob.ID, bob.Username))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /tweets 281 chars returns 400", func(t *testing.T) {
		resp := do("POST", "/tweets", jsonBody(map[string]string{"text": strings.Repeat("a", 281)}), bearerAuth(bob.ID, bob.Username))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /tweets 280 chars returns 201", func(t *testing.T) {
		resp := do("POST", "/tweets", jsonBody(map[string]string{"text": strings.Repeat("a", 280)}), bearerAuth(bob.ID, bob.Username))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("want 201, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /tweets missing auth returns 401", func(t *testing.T) {
		resp := do("POST", "/tweets", jsonBody(map[string]string{"text": "hi"}), nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	})

	// ---- GET /users/:id/tweets ----

	t.Run("GET /users/:id/tweets returns bob's tweets", func(t *testing.T) {
		resp := do("GET", "/users/"+bob.ID+"/tweets", nil, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}
		var tweets []tweetResp
		if err := json.NewDecoder(resp.Body).Decode(&tweets); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, tw := range tweets {
			if tw.ID == bobTweet.ID {
				return
			}
		}
		t.Fatalf("bob's tweet not found in response")
	})

	// Setup: alice follows bob.
	{
		resp := do("POST", "/users/"+bob.ID+"/follow", nil, bearerAuth(alice.ID, alice.Username))
		if resp.StatusCode != http.StatusNoContent {
			resp.Body.Close()
			t.Fatalf("alice follow bob: got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	// ---- POST /users/:id/follow ----

	t.Run("POST /users/:id/follow is idempotent", func(t *testing.T) {
		resp := do("POST", "/users/"+bob.ID+"/follow", nil, bearerAuth(alice.ID, alice.Username))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("want 204, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /users/:id/follow self-follow returns 405", func(t *testing.T) {
		resp := do("POST", "/users/"+alice.ID+"/follow", nil, bearerAuth(alice.ID, alice.Username))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("want 405, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /users/:id/follow missing auth returns 401", func(t *testing.T) {
		resp := do("POST", "/users/"+bob.ID+"/follow", nil, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	})

	// ---- WS /ws/timeline ----

	t.Run("WS /ws/timeline missing token returns 400", func(t *testing.T) {
		_, resp, err := websocket.DefaultDialer.Dial(wsURL(srv, "/ws/timeline"), nil)
		if err == nil {
			t.Fatal("expected connection to fail without token")
		}
		if resp == nil || resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("want 400 response, got: %v", resp)
		}
	})

	t.Run("WS /ws/timeline sends initial timeline on connect", func(t *testing.T) {
		conn, _, err := websocket.DefaultDialer.Dial(
			wsURL(srv, "/ws/timeline?token="+makeToken(t, alice.ID, alice.Username)), nil,
		)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()

		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read initial message: %v", err)
		}
		var envelope struct {
			Type string        `json:"type"`
			Data []interface{} `json:"data"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if envelope.Type != "timeline" {
			t.Fatalf("want type 'timeline', got %q", envelope.Type)
		}
		// Alice follows Bob so there should be at least one tweet (bob's initial tweet)
		if len(envelope.Data) == 0 {
			t.Fatal("expected at least one tweet in initial timeline")
		}
	})

	t.Run("WS /ws/timeline receives real-time tweet push", func(t *testing.T) {
		conn, _, err := websocket.DefaultDialer.Dial(
			wsURL(srv, "/ws/timeline?token="+makeToken(t, alice.ID, alice.Username)), nil,
		)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()

		// Discard the initial timeline message
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read initial: %v", err)
		}

		// Bob posts a new tweet after alice is connected
		resp := do("POST", "/tweets", jsonBody(map[string]string{"text": "real-time push test"}), bearerAuth(bob.ID, bob.Username))
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("bob post tweet: got %d", resp.StatusCode)
		}

		// Expect the tweet to arrive via the fan-out worker (up to 5s)
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read real-time push: %v", err)
		}

		var push struct {
			Type string `json:"type"`
			Data struct {
				Text string `json:"text"`
			} `json:"data"`
		}
		if err := json.Unmarshal(msg, &push); err != nil {
			t.Fatalf("unmarshal push: %v", err)
		}
		if push.Type != "tweet" {
			t.Fatalf("want type 'tweet', got %q", push.Type)
		}
		if push.Data.Text != "real-time push test" {
			t.Fatalf("want text 'real-time push test', got %q", push.Data.Text)
		}
	})

	// ---- DELETE /users/:id/follow ----

	t.Run("DELETE /users/:id/follow alice unfollows bob", func(t *testing.T) {
		resp := do("DELETE", "/users/"+bob.ID+"/follow", nil, bearerAuth(alice.ID, alice.Username))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("want 204, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /users/:id/follow missing auth returns 401", func(t *testing.T) {
		resp := do("DELETE", "/users/"+bob.ID+"/follow", nil, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	})
}

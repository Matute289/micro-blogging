package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"UalaTwitter/internal/docs"
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
)

func main() {
	ctx := context.Background()

	// PostgreSQL
	pgPool, err := pgxpool.New(ctx, mustEnv("POSTGRES_URL"))
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pgPool.Close()
	if err := migrate(ctx, pgPool); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// MongoDB
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mustEnv("MONGO_URL")))
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
	}
	defer mongoClient.Disconnect(ctx)
	mongoDB := mongoClient.Database(getEnv("MONGO_DB", "twitter"))

	// Redis
	redisOpts, err := redis.ParseURL(mustEnv("REDIS_URL"))
	if err != nil {
		log.Fatalf("redis url: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	// Repositories
	userRepo := userpg.NewUserRepository(pgPool)
	followRepo := followpg.NewFollowRepository(pgPool)
	tweetRepo := tweetmongo.NewTweetRepository(mongoDB)
	timelineCache := timelineredis.NewTimelineCache(redisClient)

	// WebSocket hub (implements application.Notifier)
	wsHub := timelinews.NewHub()

	// Services
	userSvc := userapp.NewUserService(userRepo)
	followSvc := followapp.NewFollowService(followRepo)
	tweetSvc := tweetapp.NewTweetService(tweetRepo)
	timelineSvc := timelineapp.NewTimelineService(followRepo, tweetRepo, timelineCache, wsHub)

	// Fan-out queue + worker
	fanOutQueue := timelinequeue.NewRedisQueue(redisClient)
	go fanOutQueue.RunWorker(context.Background(), timelineSvc.FanOutTweet)

	// Router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(corsMiddleware)
	userhandler.NewUserHandler(userSvc).RegisterRoutes(r)
	followhandler.NewFollowHandler(followSvc).RegisterRoutes(r)
	tweethandler.NewTweetHandler(tweetSvc, fanOutQueue).RegisterRoutes(r)
	timelinews.NewHandler(timelineSvc, wsHub).RegisterRoutes(r)
	docs.RegisterRoutes(r)

	addr := ":" + getEnv("PORT", "8080")
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
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
	`)
	return err
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-ID, X-Request-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s not set", key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

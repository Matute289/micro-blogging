# Simplified Twitter - High Level Architecture

## Overview

A read-optimized microblogging backend that allows users to post tweets, follow other users, and consume a personalized timeline. The system is designed to scale to millions of users with low-latency reads as the primary constraint.

---

## System Components

```
                        +------------------+
                        |   HTTP Client    |
                        +--------+---------+
                                 |
                                 v  REST / JSON
                        +--------+---------+
                        |    Go API        |  :8080
                        |  (chi router)    |
                        +--+---+---+---+---+
                           |   |   |   |
              +------------+   |   |   +---------------------+
              |                |   |                         |
              v                v   v                         v
      +-------+------+  +------+---+--+  +-------------------+--------+
      |  PostgreSQL  |  |   MongoDB   |  |         Redis               |
      |  (users +    |  |  (tweets)   |  |  timeline cache             |
      |  follows)    |  |             |  |  + fanout:queue (List)      |
      +--------------+  +-------------+  +---+------------------------+
                                             |
                                             | BLPOP (background worker)
                                             v
                                    +--------+--------+
                                    |  Fan-out Worker  |
                                    |  (goroutine)     |
                                    |  retry up to 15x |
                                    +-----------------+
```

---

## Infrastructure

| Component | Technology | Version |
|-----------|-----------|---------|
| Runtime | Go | 1.26 |
| HTTP Router | chi | 5.x |
| SQL Database | PostgreSQL | 16 |
| Document Database | MongoDB | 7 |
| Cache | Redis | 7 |
| Containerization | Docker + Compose | - |

All components run as Docker containers. A single `docker compose up --build` starts the full stack. The Go app waits for all three databases to pass their healthchecks before accepting traffic.

### Why Docker

Reproducible environment: anyone can run the full stack with one command, with no local database setup required. In a production scenario this would move to Kubernetes with horizontal pod autoscaling for the API layer and managed cloud databases.

---

## Internal Architecture: Hexagonal (Ports & Adapters)

The codebase is organized around the hexagonal architecture pattern. The core business logic (domain + application layers) has zero knowledge of databases, HTTP, or any external technology. All external dependencies are injected through interfaces (ports), and the concrete implementations (adapters) live in the infrastructure layer.

```
+---------------------------------------------------------------+
|                        Delivery Layer                         |
|              HTTP Handlers  (internal/*/delivery/http)        |
+----------------------------+----------------------------------+
                             |
+----------------------------v----------------------------------+
|                      Application Layer                        |
|              Use-case Services  (internal/*/application)      |
|                                                               |
|  UserService | FollowService | TweetService | TimelineService |
+------+--------+-------+--------+------+--------+------+-------+
       |                |                |               |
   (port)            (port)           (port)          (port)
       |                |                |               |
+------v----------------v----------------v---------------v-------+
|                    Infrastructure Layer                        |
|         Adapters  (internal/*/infrastructure/*)                |
|                                                               |
|   postgres/     postgres/      mongo/        redis/           |
|   UserRepo      FollowRepo     TweetRepo     TimelineCache     |
+---------------------------------------------------------------+
       |                |                |               |
+------v----+    +-------v----+   +-------v----+  +------v-----+
| PostgreSQL|    | PostgreSQL |   |  MongoDB   |  |   Redis    |
+-----------+    +------------+   +------------+  +------------+
```

### Layers explained

**Domain** (`internal/{domain}/domain/`)
Defines the entity structs and the repository interfaces (ports). No external imports. This is the innermost ring - it never changes because of a database or framework decision.

**Application** (`internal/{domain}/application/`)
Contains the use-case services. Each service depends only on the domain interfaces, never on concrete implementations. Business rules live here: tweet text must be non-empty and at most 280 Unicode runes (both enforced as `400 Bad Request`), the self-follow guard, the timeline fan-out logic.

**Infrastructure** (`internal/{domain}/infrastructure/`)
Concrete adapters that implement the domain ports. Each adapter knows about a specific technology (pgx for Postgres, mongo-driver for MongoDB, go-redis for Redis) and translates between domain entities and the database's native format.

**Delivery** (`internal/{domain}/delivery/http/`)
HTTP handlers. Responsible for parsing requests, calling the application service, and serializing responses. Contains no business logic. All error responses go through `pkg/httputil.WriteError`, which maps `*apperr.Error` to the correct 4xx status and logs unexpected errors as 500 via `slog` with a request ID.

### Domain breakdown

| Domain | Responsibility | Storage |
|--------|---------------|---------|
| `user` | Create and retrieve users | PostgreSQL |
| `follow` | Follow / unfollow, get social graph | PostgreSQL |
| `tweet` | Post tweets, query by user | MongoDB |
| `timeline` | Serve personalized feed, manage cache | Redis + fallback to Postgres + MongoDB |

---

## Database Design

### PostgreSQL - Relational data (users and social graph)

Chosen because user records and follow relationships are structured, require strong consistency, and benefit from ACID transactions and foreign key enforcement.

**users**
```
id           UUID        PRIMARY KEY
username     TEXT        UNIQUE NOT NULL
created_at   TIMESTAMPTZ NOT NULL
last_seen_at TIMESTAMPTZ NOT NULL
```

**follows**
```
follower_id  UUID        NOT NULL  REFERENCES users(id)
following_id UUID        NOT NULL  REFERENCES users(id)
created_at   TIMESTAMPTZ NOT NULL
PRIMARY KEY (follower_id, following_id)

INDEX idx_follows_follower  ON follows(follower_id)   -- "who do I follow?"
INDEX idx_follows_following ON follows(following_id)  -- "who follows me?"
```

The dual index is the key design decision: both directions of the graph are O(log N) lookups without joins or full scans.

### MongoDB - Document store (tweets)

Chosen because tweets have a variable-shape body (text + optional image, video, music items), there is no need for relational joins, and the collection scales horizontally by sharding on `user_id`.

**tweets collection**
```json
{
  "_id":          "a1b2c3d4e5f67890abcdef1234567890_1713434400000",
  "user_id":      "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "text":         "Hello world!",
  "media":        [{ "type": "image", "url": "https://..." }],
  "created_at_ms": 1713434400000
}
```

**Index:** `{ user_id: 1, created_at_ms: -1 }` - covers both single-user and multi-user timeline queries sorted newest first.

#### Tweet ID design

The `_id` is a composite string: `{32-char hex user UUID}_{unix milliseconds}`.

```
a1b2c3d4e5f67890abcdef1234567890_1713434400000
|_______________________________|_____________|
         user identity            timestamp
```

This encodes both authorship and creation time directly in the ID. Decomposing it (splitting on `_`) instantly yields the author UUID and a `time.Time` without any extra database lookup. It also serves as a natural cursor for keyset pagination.

### Redis - Timeline cache

Chosen for its sorted set data structure, which maps directly to the timeline use case: tweet IDs ranked by timestamp, with O(log N) insert and O(log N + K) range read.

**Key:** `timeline:{user_id}`
**Type:** Sorted Set
**Score:** Unix milliseconds (allows range queries and cursor pagination)
**Member:** tweet ID (composite ID, used to fetch the full document from MongoDB)

```
ZADD  timeline:alice  1713434460000  "bob_tweet_id_2"
ZADD  timeline:alice  1713434400000  "bob_tweet_id_1"

ZREVRANGEBYSCORE timeline:alice +inf -inf LIMIT 0 20
  -> ["bob_tweet_id_2", "bob_tweet_id_1"]
```

Each timeline is capped at 200 entries (`ZREMRANGEBYRANK` after each insert) to bound memory usage.

---

## Key Flows

### 1. Post a tweet (write path)

```
Client
  |
  | POST /tweets  (X-User-ID: alice)
  v
HTTP Handler
  |
  | TweetService.PostTweet()
  v
MongoDB  <-- Save tweet document
  |
  | (success)
  v
HTTP Handler
  |
  | queue.Enqueue()  [Redis RPUSH to fanout:queue - fast, non-blocking]
  v
Redis  <-- RPUSH fanout:queue {poster_id, tweet_id, created_at}
  |
  v
HTTP Handler  --> 201 response to client


[Background Fan-out Worker - running as goroutine in main.go]
  |
  | BLPOP fanout:queue (blocking, 1s timeout)
  v
PostgreSQL  <-- GetFollowers(alice)
  |
  | for each follower with a warm Redis cache:
  v
Redis  <-- ZADD timeline:{follower} score tweetID
           ZREMRANGEBYRANK (trim to 200)
  |
  | on PG or Redis error: requeue with attempt++ (up to 15 retries)
  | after 15 failures: log error and drop
```

The tweet is saved to MongoDB and the fan-out job is enqueued before the `201` is sent. The actual cache update happens asynchronously in the worker goroutine. On transient infrastructure failures the message is requeued and retried, making the fan-out resilient without affecting write latency.

### 2. Get timeline (read path - cache hit)

```
Client
  |
  | GET /timeline  (X-User-ID: alice)
  v
HTTP Handler
  |
  | TimelineService.GetTimeline()
  v
Redis  <-- ZREVRANGEBYSCORE timeline:alice +inf -inf LIMIT 0 20
  |
  | [tweet_id_1, tweet_id_2, ...]
  v
MongoDB  <-- find({ _id: { $in: [ids] } })
  |
  | tweet documents
  v
HTTP Handler  --> 200 JSON response
```

Total: 2 database roundtrips, both to in-memory or indexed stores.

### 3. Get timeline (read path - cache miss / cold start)

```
Client
  |
  | GET /timeline  (X-User-ID: alice)
  v
TimelineService
  |
  | Redis key does not exist
  v
PostgreSQL  <-- SELECT following_id FROM follows WHERE follower_id = alice
  |
  | [bob, carol, ...]
  v
MongoDB  <-- find({ user_id: { $in: [bob, carol] } }).sort(-created_at_ms).limit(20)
  |
  | tweet documents
  v
TimelineService  --> ZADD each tweet into Redis (warms cache for next request)
  |
  v
HTTP Handler  --> 200 JSON response
```

### 4. Follow a user

```
Client
  |
  | POST /users/{id}/follow  (X-User-ID: alice)
  v
FollowService.Follow()
  |
  | self-follow guard: alice == {id}? -> 405 Method Not Allowed
  v
PostgreSQL  <-- INSERT INTO follows (follower_id, following_id) ON CONFLICT DO NOTHING
  |
  v
HTTP Handler  --> 204 No Content
```

---

## Read Optimization Strategy

The system is designed for a high read-to-write ratio, matching the real Twitter usage pattern.

**Fan-out on write via async queue** is the core strategy: when a tweet is posted, a message is enqueued to a Redis List (`fanout:queue`). A background worker dequeues it and pushes the tweet ID into every follower's Redis timeline sorted set. The read path then only needs two fast lookups (Redis range + MongoDB `$in` by ID) regardless of how many users the caller follows. Transient failures in the worker are retried up to 15 times before the message is dropped.

| Operation | Data store hit | Complexity |
|-----------|---------------|------------|
| GET /timeline (cache hit) | Redis + MongoDB | O(log N + K) |
| GET /timeline (cache miss) | PostgreSQL + MongoDB + Redis | O(F log N + K) |
| POST /tweets | MongoDB + Redis (x followers) | O(F log N) write amplification |
| POST /follow | PostgreSQL | O(log N) |

Where N = timeline size, K = result count, F = follower count.

**Trade-off acknowledged:** for accounts with very large follower counts (celebrities with millions of followers), fan-out write amplification grows linearly with follower count. The current worker processes all followers sequentially in a single goroutine. The production solution is a hybrid approach: fan-out on write for regular users, fan-out on read for high-follower accounts, with multiple parallel worker instances consuming from the queue (or migrating to Kafka for higher throughput).

---

## Scalability Path

The current implementation runs as a single Docker Compose stack. The design allows the following production evolution without changing the business logic:

```
                    [ Load Balancer ]
                     /      |      \
              [API]       [API]       [API]     <- stateless, scale horizontally
                |           |           |
         [PostgreSQL]  [MongoDB]    [Redis]
          Primary +     Sharded      Cluster
          Replicas      by user_id
```

- **API layer:** stateless, can run N replicas behind any load balancer.
- **PostgreSQL:** read replicas for follow-graph queries; primary for writes only.
- **MongoDB:** shard key on `user_id` distributes tweets evenly across shards. The compound index `{user_id, created_at_ms}` remains local to each shard.
- **Redis:** Redis Cluster for timeline cache; consistent hashing routes each `timeline:{user_id}` key to the same shard.
- **Fan-out at scale:** introduce a message queue (Kafka topic `tweet.created`) between the write path and the fan-out workers. Multiple consumer instances process fan-out in parallel.

---

## Technology Choice Summary

| Technology | Why chosen |
|-----------|-----------|
| **Go** | High throughput, low memory footprint, built-in concurrency (goroutines for async fan-out), single statically linked binary |
| **chi** | Lightweight HTTP router, zero dependencies beyond stdlib, idiomatic middleware chaining |
| **PostgreSQL** | ACID guarantees for user and follow data; efficient dual-direction index on the follows table |
| **MongoDB** | Flexible document model fits variable tweet body (text + any media mix); scales horizontally by sharding |
| **Redis** | Sorted sets are a natural fit for the ranked timeline; sub-millisecond reads; built-in TTL and size trimming |
| **Docker Compose** | One-command reproducible environment; trivial path to Kubernetes in production |
| **Hexagonal architecture** | Business logic is isolated from infrastructure; each adapter can be swapped (e.g., swap Postgres for CockroachDB) without touching a single service or domain file |

---

## Project Structure

```
UalaTwitter/
  cmd/api/main.go                  <- entry point, dependency wiring, DB migrations
  internal/
    user/
      domain/                      <- User entity, UserRepository interface
      application/                 <- UserService (create, get)
      infrastructure/postgres/     <- pgx adapter
      delivery/http/               <- HTTP handler
    follow/
      domain/                      <- Follow entity, FollowRepository interface
      application/                 <- FollowService (follow, unfollow, graph queries)
      infrastructure/postgres/     <- pgx adapter
      delivery/http/               <- HTTP handler
    tweet/
      domain/                      <- Tweet entity, TweetRepository interface
      application/                 <- TweetService (post, list), 280-char guard
      infrastructure/mongo/        <- mongo-driver adapter
      delivery/http/               <- HTTP handler
    timeline/
      application/                 <- TimelineService (get timeline, fan-out)
      infrastructure/redis/        <- Redis sorted-set cache adapter
      infrastructure/queue/        <- Redis List queue + RunWorker (fan-out with retry)
      delivery/http/               <- HTTP handler
    docs/
      openapi.yaml                 <- OpenAPI 3.0.3 spec (embedded into binary)
      handler.go                   <- serves GET /openapi.yaml and GET /swagger
  pkg/
    tweetid/                       <- Compose / Decompose tweet ID
    apperr/                        <- Typed errors with HTTP status (Invalid, NotFound, Conflict, NotAllowed)
    httputil/                      <- WriteError: 4xx from apperr, 500 + slog for everything else
  Dockerfile
  docker-compose.yml
  business.txt                     <- additional assumptions
  README.md                        <- setup and API reference
```

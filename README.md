# UalaTwitter

A simplified Twitter-like microblogging backend built in Go.

## Stack

- **Go** - HTTP API (chi router, hexagonal architecture)
- **PostgreSQL** - users and follow graph
- **MongoDB** - tweet storage
- **Redis** - timeline cache (fan-out on write)

---

## Running the project

### Requirements

- [Docker](https://www.docker.com/) and Docker Compose

### Start everything

```bash
docker compose up --build
```

This starts the app on port `8080` plus PostgreSQL, MongoDB, and Redis. The database schema is created automatically on startup.

### API documentation

Once the stack is running, the interactive API reference is available at:

| URL | Content |
|-----|---------|
| http://localhost:8080/swagger | Swagger UI (interactive) |
| http://localhost:8080/openapi.yaml | Raw OpenAPI 3.0.3 spec |

### Stop

```bash
docker compose down
```

---

## Testing

### Unit tests

No running services required.

```bash
go test ./...
```

### Integration tests

Requires PostgreSQL, MongoDB, and Redis to be running. The easiest way is to start the stack first:

```bash
docker compose up -d
```

Then run:

```bash
go test -tags integration -v ./test/...
```

The test wires the full application in-process using `httptest.Server` and exercises every endpoint end-to-end. Each run uses unique usernames so it is safe to run multiple times against the same databases.

---

## Authentication

There is no login. Pass the caller's user ID in every request that requires identity:

```
X-User-ID: <user-uuid>
```

---

## Error responses

All errors return a plain-text body and an HTTP status code.

| Status | Meaning |
|--------|---------|
| `400 Bad Request` | Invalid input (missing field, self-follow, text too long, missing header) |
| `404 Not Found` | Resource does not exist |
| `409 Conflict` | Duplicate resource |
| `500 Internal Server Error` | Unexpected server-side failure - see server logs for details |

```
HTTP/1.1 400 Bad Request
Content-Type: text/plain; charset=utf-8

cannot follow yourself
```

Server errors never expose internal details to the client. Each `500` is logged server-side with a `Request-Id` header that matches the log entry.

---

## Endpoints

### Create a user

```
POST /users
```

**Request**
```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"username": "alice"}'
```

**Response** `201 Created`
```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "username": "alice",
  "created_at": "2026-04-18T10:00:00Z",
  "last_seen_at": "2026-04-18T10:00:00Z"
}
```

**Errors**

| Status | Body |
|--------|------|
| `400` | `username required` |

---

### Get a user

```
GET /users/{id}
```

**Request**
```bash
curl http://localhost:8080/users/a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

**Response** `200 OK`
```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "username": "alice",
  "created_at": "2026-04-18T10:00:00Z",
  "last_seen_at": "2026-04-18T10:00:00Z"
}
```

**Errors**

| Status | Body |
|--------|------|
| `404` | `not found` |

---

### Post a tweet

```
POST /tweets
Header: X-User-ID
```

Text is required and limited to 280 characters (Unicode runes). Media items are URL references (image, video, or music).

**Request**
```bash
curl -X POST http://localhost:8080/tweets \
  -H "Content-Type: application/json" \
  -H "X-User-ID: a1b2c3d4-e5f6-7890-abcd-ef1234567890" \
  -d '{
    "text": "Hello world!",
    "media": [
      { "type": "image", "url": "https://example.com/photo.jpg" }
    ]
  }'
```

**Request (text only)**
```bash
curl -X POST http://localhost:8080/tweets \
  -H "Content-Type: application/json" \
  -H "X-User-ID: a1b2c3d4-e5f6-7890-abcd-ef1234567890" \
  -d '{"text": "Just a text tweet"}'
```

**Response** `201 Created`
```json
{
  "id": "a1b2c3d4e5f67890abcdef1234567890_1713434400000",
  "user_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "text": "Hello world!",
  "media": [
    { "type": "image", "url": "https://example.com/photo.jpg" }
  ],
  "created_at": "2026-04-18T10:00:00Z"
}
```

> The `id` encodes both the author and the creation timestamp, so no extra fields are needed to determine when or by whom a tweet was created.

**Errors**

| Status | Body |
|--------|------|
| `400` | `missing X-User-ID header` |
| `400` | `tweet text cannot be empty` |
| `400` | `text exceeds 280 characters` |

---

### Get a user's tweets

```
GET /users/{id}/tweets
```

| Query param | Description | Default |
|-------------|-------------|---------|
| `limit` | Number of tweets to return (max 50) | 20 |
| `before` | Tweet ID cursor - returns tweets older than this | - |

**Request**
```bash
curl "http://localhost:8080/users/a1b2c3d4-e5f6-7890-abcd-ef1234567890/tweets?limit=5"
```

**Response** `200 OK`
```json
[
  {
    "id": "a1b2c3d4e5f67890abcdef1234567890_1713434400000",
    "user_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "text": "Hello world!",
    "media": [
      { "type": "image", "url": "https://example.com/photo.jpg" }
    ],
    "created_at": "2026-04-18T10:00:00Z"
  }
]
```

**Paginate to next page** - pass the last tweet's `id` as cursor:
```bash
curl "http://localhost:8080/users/a1b2c3d4-e5f6-7890-abcd-ef1234567890/tweets?limit=5&before=a1b2c3d4e5f67890abcdef1234567890_1713434400000"
```

---

### Follow a user

```
POST /users/{id}/follow
Header: X-User-ID
```

**Request**
```bash
curl -X POST http://localhost:8080/users/b2c3d4e5-f6a7-8901-bcde-f12345678901/follow \
  -H "X-User-ID: a1b2c3d4-e5f6-7890-abcd-ef1234567890"
```

**Response** `204 No Content`

**Errors**

| Status | Body |
|--------|------|
| `400` | `missing X-User-ID header` |
| `400` | `cannot follow yourself` |

---

### Unfollow a user

```
DELETE /users/{id}/follow
Header: X-User-ID
```

**Request**
```bash
curl -X DELETE http://localhost:8080/users/b2c3d4e5-f6a7-8901-bcde-f12345678901/follow \
  -H "X-User-ID: a1b2c3d4-e5f6-7890-abcd-ef1234567890"
```

**Response** `204 No Content`

**Errors**

| Status | Body |
|--------|------|
| `400` | `missing X-User-ID header` |

---

### Get timeline

Returns tweets from all users the caller follows, sorted newest first.

```
GET /timeline
Header: X-User-ID
```

| Query param | Description | Default |
|-------------|-------------|---------|
| `limit` | Number of tweets to return (max 50) | 20 |
| `before` | Tweet ID cursor - returns tweets older than this | - |

**Request**
```bash
curl http://localhost:8080/timeline \
  -H "X-User-ID: a1b2c3d4-e5f6-7890-abcd-ef1234567890"
```

**Response** `200 OK`
```json
[
  {
    "id": "b2c3d4e5f6a78901bcdeff12345678901_1713434460000",
    "user_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "text": "Hey from bob!",
    "created_at": "2026-04-18T10:01:00Z"
  },
  {
    "id": "b2c3d4e5f6a78901bcdeff12345678901_1713434400000",
    "user_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "text": "First tweet",
    "created_at": "2026-04-18T10:00:00Z"
  }
]
```

**Paginate to next page:**
```bash
curl "http://localhost:8080/timeline?limit=20&before=b2c3d4e5f6a78901bcdeff12345678901_1713434400000" \
  -H "X-User-ID: a1b2c3d4-e5f6-7890-abcd-ef1234567890"
```

**Errors**

| Status | Body |
|--------|------|
| `400` | `missing X-User-ID header` |

---

## Quick start example

```bash
# 1. Start the stack
docker compose up --build -d

# 2. Create two users
ALICE=$(curl -s -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"username":"alice"}' | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

BOB=$(curl -s -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"username":"bob"}' | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

# 3. Alice follows Bob
curl -X POST http://localhost:8080/users/$BOB/follow \
  -H "X-User-ID: $ALICE"

# 4. Bob posts a tweet
curl -X POST http://localhost:8080/tweets \
  -H "Content-Type: application/json" \
  -H "X-User-ID: $BOB" \
  -d '{"text":"Hello from Bob!"}'

# 5. Alice reads her timeline
curl http://localhost:8080/timeline \
  -H "X-User-ID: $ALICE"
```

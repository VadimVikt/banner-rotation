# Banner Rotation Service

A REST API service for rotating banners using a multi-armed bandit algorithm (Thompson Sampling). Banners are assigned to **slots** and selection is personalized per **socio-demographic group**. Impression and click events are published to a message queue (RabbitMQ) for downstream analytics.

## Architecture

| Layer | Package | Dependencies |
|-------|---------|--------------|
| Core (bandit) | `internal/bandit` | stdlib only |
| Core (service) | `internal/service` | stdlib only |
| Core (model) | `internal/model` | stdlib only |
| Infra (repo) | `internal/repo` | SQLite (`modernc.org/sqlite`) |
| Infra (event) | `internal/event` | RabbitMQ (`streadway/amqp`) |
| Infra (handler) | `internal/handler` | `go-chi/chi/v5` |

## Requirements

- Go ≥ 1.25
- Docker + Docker Compose (for `make run`)

## Build & Run

```bash
# Build binary
make build

# Run full stack (app + RabbitMQ) via Docker Compose
make run

# Stop
docker compose down
```

The service listens on `:8080` by default.

## Tests

```bash
# Run all tests with race detector (100 iterations)
make test
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/slots` | Create a slot |
| `POST` | `/slots/{slotID}/banners` | Add a banner to a slot |
| `DELETE` | `/slots/{slotID}/banners/{bannerID}` | Remove a banner from a slot |
| `POST` | `/slots/{slotID}/pick?groupID=...` | Pick a banner (bandit selection) |
| `POST` | `/slots/{slotID}/banners/{bannerID}/click?groupID=...` | Register a click |

### Request/Response Examples

**Create slot:**
```bash
curl -X POST http://localhost:8080/slots \
  -H 'Content-Type: application/json' \
  -d '{"id": "homepage-top", "description": "Main page top banner"}'
```

**Add banner:**
```bash
curl -X POST http://localhost:8080/slots/homepage-top/banners \
  -H 'Content-Type: application/json' \
  -d '{"banner_id": "promo-summer", "description": "Summer sale"}'
```

**Pick banner:**
```bash
curl -X POST 'http://localhost:8080/slots/homepage-top/pick?groupID=18-25'
# → {"banner_id":"promo-summer"}
```

**Register click:**
```bash
curl -X POST 'http://localhost:8080/slots/homepage-top/banners/promo-summer/click?groupID=18-25'
# → 204 No Content
```

## Configuration

The server accepts flags and environment variables:

| Flag / Env | Default | Description |
|------------|---------|-------------|
| `-addr` / — | `:8080` | HTTP listen address |
| `-db` / — | `banner_rotation.db` | SQLite database file path |
| `-rabbitmq` / `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | RabbitMQ connection URL; use `nop` to disable event publishing |

## Project Structure

```
├── cmd/server/main.go        # Entry point
├── internal/
│   ├── bandit/               # Thompson Sampling (core)
│   ├── model/                # Domain types (core)
│   ├── service/              # Business logic (core)
│   ├── repo/                 # SQLite repository (infra)
│   ├── event/                # Event publisher — RabbitMQ / NOP (infra)
│   └── handler/              # HTTP handlers (infra)
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── go.mod
```

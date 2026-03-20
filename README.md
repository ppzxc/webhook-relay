# relaybox

Generic relay hub: receives any inbound protocol/format and delivers to outbound channels (CEL/Expr expression filter/transform/route rules planned).

```
any inbound (HTTP REST / WebSocket / TCP / ...)
        ↓
  CEL / Expr expression filter + transform + route
        ↓
any outbound (Webhook / Slack / Discord / ...)
```

## Features

- **Multi-protocol inbound** — HTTP REST + WebSocket (TCP planned)
- **Expression-based routing** — CEL/Expr filter and transform rules per route (planned)
- **Template transformation** — Go `text/template` payload rendering
- **at-least-once delivery** — file-queue backed, survives restarts
- **Exponential backoff retry** — per-channel `retryCount` / `retryDelayMs`
- **Hot config reload** — change outputs / rules without restart
- **Bearer token auth** — per-input independent secrets

## Quick Start

### Prerequisites

- Go 1.25+
- GCC (required for go-sqlite3 CGO build)

```bash
# Build
CGO_ENABLED=1 go build -o relaybox ./cmd/server/

# Prepare config
cp internal/config/config.example.yaml config.yaml
# Edit config.yaml, then:

# Start server
./relaybox start --config config.yaml
```

## Configuration

`config.yaml` example:

```yaml
server:
  port: 8080

log:
  level: info    # debug | info | warn | error
  format: json   # json | text

inputs:
  - id: beszel
    type: BESZEL
    secret: "your-secret"

outputs:
  - id: ops-webhook
    type: WEBHOOK
    url: "https://hooks.example.com/xyz"
    template: '{"text": "{{ .Source }}: {{ .Payload }}"}'
    retryCount: 3
    retryDelayMs: 1000

rules:
  - inputId: beszel
    outputIds: [ops-webhook]

storage:
  type: SQLITE
  path: "./data/relaybox.db"

queue:
  type: FILE
  path: "./data/queue"
  workerCount: 2
```

### Template Variables

| Variable | Description |
|----------|-------------|
| `{{ .ID }}` | Alert ULID |
| `{{ .Source }}` | Source type (`BESZEL`, `DOZZLE`, etc.) |
| `{{ .Payload }}` | Raw JSON payload (string) |
| `{{ .CreatedAt }}` | Receive time (`time.Time`) |

## API

### Receive Message

```
POST /inputs/{inputId}/messages
Authorization: Bearer <secret>
Content-Type: application/json

{"host": "server1", "status": "down"}
```

Response `201 Created`:
```json
{"id": "01J...", "status": "PENDING"}
```

### WebSocket Inbound

```
GET /inputs/{inputId}/messages/ws
Authorization: Bearer <secret>
```

JSON messages sent after connect are processed identically to HTTP POST.

### Health Check

```
GET /healthz
→ 200 OK
```

All responses include an `X-API-Version` header.

## Architecture

Hexagonal architecture (Ports & Adapters). Dependencies always flow inward toward domain.

```
domain (0 deps)
  ↑
application/port/{input,output}  ← interface definitions
  ↑
application/service              ← business logic
  ↑
adapter/{input,output}           ← external world connections
  ↑
cmd/server/main.go               ← DI wiring, cobra CLI
```

## Development

```bash
# Full test suite (race detector)
go test -race ./... -timeout 60s

# Static analysis
go vet ./...

# Regenerate sqlc (after SQL changes)
cd internal/adapter/output/sqlite && sqlc generate
```

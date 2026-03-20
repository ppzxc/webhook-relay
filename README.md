# relaybox

Generic relay hub: receives any inbound protocol/format and delivers to outbound channels via CEL/Expr expression filter, transform, and route rules.

```
any inbound (HTTP REST / WebSocket / TCP / ...)
        ↓
  parser pipeline (JSON / Form / XML / Logfmt / Regex)
        ↓
  CEL / Expr expression filter + transform + route
        ↓
any outbound (Webhook / Slack / Discord / ...)
```

## Features

- **Multi-protocol inbound** — HTTP REST + WebSocket + TCP
- **Parser pipeline** — JSON, Form, XML, Logfmt, Regex per input
- **Expression-based routing** — CEL/Expr filter, mapping, and routing conditions per rule
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
  readTimeout: 30s
  writeTimeout: 30s

log:
  level: info    # debug | info | warn | error
  format: json   # json | text

expression:
  defaultEngine: cel  # cel | expr

inputs:
  - id: beszel
    type: BESZEL
    parser: json      # json | form | xml | logfmt | regex
    secret: "your-secret"
  - id: tcp-input
    type: GENERIC
    address: ":9001"
    delimiter: "\n"
    parser: json
    secret: ""        # secret unused for TCP inputs

outputs:
  - id: ops-webhook
    type: WEBHOOK
    url: "https://hooks.example.com/xyz"
    template:
      text: 'data.input + ": " + data.payload'
    retryCount: 3
    retryDelayMs: 1000

rules:
  - inputId: beszel
    engine: cel           # override default engine per rule
    filter: 'data.input == "BESZEL"'
    mapping:
      severity: '"HIGH"'
    routing:
      - condition: 'data.severity == "HIGH"'
        outputIds: [ops-webhook]
  - inputId: tcp-input
    outputIds: [ops-webhook]  # simple: no filter/routing, send to all

storage:
  type: SQLITE
  path: "./data/relaybox.db"

queue:
  type: FILE
  path: "./data/queue"
  workerCount: 2
```

### Expression Variables

All expressions (filter, mapping, routing, template) share the same `data` context:

| Variable | Description |
|----------|-------------|
| `data.id` | Message ULID |
| `data.input` | Input type (`BESZEL`, `DOZZLE`, `GENERIC`, etc.) |
| `data.payload` | Raw payload string |
| `data.created_at` | Receive timestamp |
| `data.<field>` | Any field added via `mapping` expressions |

**Filter** — boolean expression; message is dropped if `false`:
```yaml
filter: 'data.input == "BESZEL"'
```

**Mapping** — enrich `data` with computed fields:
```yaml
mapping:
  severity: '"HIGH"'
  label: 'data.input + "-alert"'
```

**Routing** — conditional output selection (evaluated after mapping):
```yaml
routing:
  - condition: 'data.severity == "HIGH"'
    outputIds: [ops-webhook]
```

**Template** — map of output fields rendered as expressions:
```yaml
template:
  text: 'data.input + ": " + data.payload'
```

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

### TCP Inbound

Connect to the configured `address` and send newline-delimited (or custom `delimiter`) messages. No token auth — secure via network policy.

### Health Check

```
GET /healthz
→ 200 OK
```

All HTTP responses include an `X-API-Version` header.

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

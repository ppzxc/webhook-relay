**English** | [한국어](README.ko.md)

# relaybox

A generic relay hub: receives any inbound protocol/format, applies CEL/Expr expression-based filter, transform, and routing rules, then forwards messages to outbound channels.

```
Any inbound (HTTP REST / WebSocket / TCP / ...)
        ↓
  Parser pipeline (JSON / Form / XML / Logfmt / Regex)
        ↓
  CEL / Expr expression filter + transform + routing
        ↓
Any outbound (Webhook / ...)
```

## Features

- **Multi-protocol inbound** — HTTP REST, WebSocket, TCP
- **Parser pipeline** — JSON, Form, XML, Logfmt, Regex (with custom pattern) per input
- **Expression-based routing** — per-input CEL/Expr filter, mapping, and routing rules
- **At-least-once delivery** — file-queue backed; messages survive restarts
- **Exponential backoff retry** — per-output `retryCount` / `retryDelayMs`
- **Config hot-reload** — change outputs and rules without restarting
- **Bearer token auth** — per-input independent secret
- **Dot-notation templates** — produce nested JSON output via `parent.child` keys

## Quick Start

### Prerequisites

- Go 1.25+

### Build

```bash
# Clone and build
go build -o relaybox ./cmd/server/

# Copy example config
cp docs/config.example.yaml config.yaml
# Edit config.yaml, then:

# Start server
./relaybox start --config config.yaml
```

### Makefile

```bash
# Build for current platform
make build

# Cross-compile all platforms (output to dist/)
make build-all

# Run tests
make test

# Full release build (clean + build-all + checksums)
make release VERSION=1.0.0
```

## Configuration

### Quick Example

`config.yaml` example (see `docs/config.example.yaml` for full reference):

```yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
  tls:
    enabled: false
    certFile: ""
    keyFile: ""

log:
  level: INFO    # DEBUG, INFO, WARN, ERROR
  format: JSON   # JSON, TEXT

inputs:
  - id: beszel
    engine: CEL          # required — CEL or EXPR
    parser: JSON         # JSON, FORM, XML, LOGFMT, REGEX
    secret: "change-me"
    rules:
      # Rule 1: conditional routing
      - filter: 'data.severity == "HIGH"'
        mapping:
          level: '"CRITICAL"'
        routing:
          - condition: 'data.level == "CRITICAL"'
            outputIds: [ops-webhook]
      # Rule 2: always forward (no filter)
      - outputIds: [notify-bot]

  - id: tcp-input
    engine: CEL
    address: ":9001"
    delimiter: "\n"
    parser: JSON
    rules:
      - outputIds: [ops-webhook]   # simple: no filter, send all

outputs:
  - id: ops-webhook
    type: WEBHOOK
    engine: CEL          # required — CEL or EXPR
    url: "https://hooks.example.com/xyz"
    template:
      text: 'data.input + ": " + data.payload'
    retryCount: 3
    retryDelayMs: 1000
    skipTLSVerify: false

  # Dot-notation keys produce nested JSON
  - id: notify-bot
    type: WEBHOOK
    engine: CEL
    url: "https://example.com/api/v1/bots/1/text"
    secret: "bearer-token"   # sent as Authorization: Bearer <secret>
    template:
      content.type: '"text"'
      content.text: 'data.input + " alert: " + data.payload'
    retryCount: 3
    retryDelayMs: 1000
    timeoutSec: 10

storage:
  type: SQLITE
  path: "./data/relaybox.db"

queue:
  type: FILE
  path: "./data/queue"
  workerCount: 2

worker:
  defaultRetryCount: 3      # fallback when output has no retryCount
  defaultRetryDelay: "1s"   # fallback base retry delay (Go duration)
  pollBackoff: "500ms"      # sleep between empty-queue polls
```

### Configuration Reference

#### `server`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `port` | int | No | `8080` | HTTP server listen port |
| `readTimeout` | duration | No | `30s` | Maximum time to read a full request. Go duration string (e.g. `10s`, `1m`). |
| `writeTimeout` | duration | No | `30s` | Maximum time to write a full response. Go duration string. |

#### `server.tls`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | bool | No | `false` | Enable TLS. When `true`, `certFile` and `keyFile` must be set. |
| `certFile` | string | No | `""` | Path to PEM-encoded TLS certificate file. |
| `keyFile` | string | No | `""` | Path to PEM-encoded TLS private key file. |

#### `log`

| Field | Type | Required | Default | Values | Description |
|-------|------|----------|---------|--------|-------------|
| `level` | string | No | `INFO` | `DEBUG`, `INFO`, `WARN`, `ERROR` | Minimum log level to emit. `DEBUG` includes all levels; `ERROR` emits only errors. |
| `format` | string | No | `JSON` | `JSON`, `TEXT` | Log output format. `JSON` outputs structured JSON lines; `TEXT` outputs human-readable key=value pairs. |

#### `inputs[]`

Each entry in the `inputs` list defines one inbound endpoint.

| Field | Type | Required | Default | Values | Description |
|-------|------|----------|---------|--------|-------------|
| `id` | string | **Yes** | — | Any unique string | Identifier used in API URLs (`/inputs/{id}/messages`). Must be unique across all inputs. Stored on every message as `data.input`. |
| `engine` | string | **Yes** | — | `CEL`, `EXPR` | Expression engine for evaluating filter/mapping/routing rules on this input. See [Expression Engine Reference](#expression-engine-reference). |
| `parser` | string | No | `JSON` | `JSON`, `FORM`, `XML`, `LOGFMT`, `REGEX` | Parser applied to the raw request body before expression evaluation. See [Parser Reference](#parser-reference). |
| `secret` | string | No | `""` | Any string | Bearer token required in `Authorization: Bearer <secret>`. If empty, all requests are accepted without auth. |
| `address` | string | No | `""` | e.g. `:9001` | TCP listen address. Only applies to TCP inputs. If set, relaybox listens for TCP connections at this address. Not used for HTTP/WebSocket inputs. |
| `delimiter` | string | No | `"\n"` | Any string | Record delimiter for TCP streams. Messages are split on this string. Only applies to TCP inputs. |
| `pattern` | string | No | `""` | RE2 regex | Named capture group regex for the `REGEX` parser. Required when `parser: REGEX`. Example: `(?P<level>\w+) (?P<msg>.+)`. |
| `rules` | list | No | `[]` | — | Ordered list of routing rules evaluated for each received message. See `inputs[].rules[]`. |

#### `inputs[].rules[]`

Rules are evaluated in order. A rule matches when its `filter` expression returns `true` (or when `filter` is absent). When a rule matches, its `mapping` is applied and outputs are selected via `outputIds` or `routing`.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `filter` | string | No | `""` (always true) | CEL/Expr boolean expression. If `false`, this rule is skipped for the current message. If absent, the rule always matches. |
| `mapping` | map[string]string | No | `{}` | Key-value pairs where each value is a CEL/Expr expression. Evaluated results are added to `data` before `routing` is evaluated. Existing fields can be overwritten. |
| `outputIds` | list[string] | No | `[]` | Static list of output IDs to send to when this rule matches and no `routing` condition fires. Acts as a fallback. |
| `routing` | list | No | `[]` | Conditional output selection evaluated after `mapping`. See `inputs[].rules[].routing[]`. |

#### `inputs[].rules[].routing[]`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `condition` | string | No | `""` (always true) | CEL/Expr boolean expression evaluated against `data` (including mapped fields). If `true`, the listed `outputIds` receive the message. |
| `outputIds` | list[string] | **Yes** | — | Output IDs to send to when `condition` is true. |

#### `outputs[]`

Each entry defines one outbound destination.

| Field | Type | Required | Default | Values | Description |
|-------|------|----------|---------|--------|-------------|
| `id` | string | **Yes** | — | Any unique string | Referenced by `outputIds` in rules. Must be unique across all outputs. |
| `type` | string | **Yes** | — | `WEBHOOK` | Output type. Currently only `WEBHOOK` is supported. |
| `engine` | string | **Yes** | — | `CEL`, `EXPR` | Expression engine used to evaluate `template` expressions. See [Expression Engine Reference](#expression-engine-reference). |
| `url` | string | **Yes** (WEBHOOK) | — | HTTP/HTTPS URL | Destination URL for webhook delivery. |
| `template` | map[string]string | No | `{}` | — | Key-value pairs where each value is a CEL/Expr expression rendered into the outbound JSON body. Dot-notation keys (e.g. `content.type`) produce nested JSON objects. |
| `secret` | string | No | `""` | Any string | If set, sent as `Authorization: Bearer <secret>` in the outbound webhook request. |
| `retryCount` | int | No | `0` (uses `worker.defaultRetryCount`) | `0`–N | Number of retry attempts after the first failure. `0` means use the worker-level default. Retries use exponential backoff starting at `retryDelayMs`. |
| `retryDelayMs` | int | No | `0` (uses `worker.defaultRetryDelay`) | `0`–N | Base delay in milliseconds between retries. Each retry doubles the delay. `0` means use the worker-level default. |
| `timeoutSec` | int | No | `0` (no timeout) | `0`–N | HTTP request timeout in seconds. `0` means no timeout. |
| `skipTLSVerify` | bool | No | `false` | `true`, `false` | Skip TLS certificate verification for HTTPS destinations. Use only in development or with self-signed certificates. |

#### `storage`

| Field | Type | Required | Default | Values | Description |
|-------|------|----------|---------|--------|-------------|
| `type` | string | **Yes** | — | `SQLITE` | Storage backend type. Currently only `SQLITE` is supported. |
| `path` | string | **Yes** | — | File path | Path to the SQLite database file. Created automatically if it does not exist. Example: `./data/relaybox.db`. |

#### `queue`

| Field | Type | Required | Default | Values | Description |
|-------|------|----------|---------|--------|-------------|
| `type` | string | **Yes** | — | `FILE` | Queue backend type. Currently only `FILE` is supported. Messages are persisted as JSON files and survive restarts. |
| `path` | string | **Yes** | — | Directory path | Directory where queued message files are stored. Created automatically if it does not exist. Example: `./data/queue`. |
| `workerCount` | int | No | `1` | `1`–N | Number of concurrent relay worker goroutines processing the queue. Higher values increase throughput but consume more CPU/memory. |

#### `worker`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `defaultRetryCount` | int | No | `3` | Fallback retry count used when an output's `retryCount` is `0` or not set. If set to `0` or negative, no retries are performed. |
| `defaultRetryDelay` | duration | No | `1s` | Fallback base retry delay used when an output's `retryDelayMs` is `0` or not set. Go duration string (e.g. `500ms`, `2s`). |
| `pollBackoff` | duration | No | `500ms` | Sleep duration between queue polls when the queue is empty. Lower values reduce latency but increase CPU usage. Go duration string. |

### Expression Variables

All expressions (filter, mapping, routing, template) share the same `data` context:

| Variable | Type | Description |
|----------|------|-------------|
| `data.id` | string | Message ULID — unique identifier assigned at receive time |
| `data.input` | string | Input ID value (e.g. `beszel`, `dozzle`) |
| `data.payload` | string | Raw payload string as received |
| `data.createdAt` | string | Receive timestamp in RFC3339 format |
| `data.<field>` | any | Fields added or overwritten by `mapping` expressions |

**Filter** — boolean expression; `false` drops the message for this rule:
```yaml
filter: 'data.input == "beszel"'
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

**Template** — render output fields with expressions. Dot-notation keys generate nested JSON:
```yaml
template:
  text: 'data.input + ": " + data.payload'
  content.type: '"text"'
  content.text: 'data.payload'
```

### Expression Engine Reference

| Engine | Value | Library | Description |
|--------|-------|---------|-------------|
| CEL | `CEL` | [google/cel-go](https://github.com/google/cel-go) | Google Common Expression Language. Strongly typed, compiled at startup. Recommended for production — compile errors are caught at startup, not at runtime. |
| Expr | `EXPR` | [expr-lang/expr](https://github.com/expr-lang/expr) | Lightweight expression evaluator. Duck-typed, simpler syntax. Suitable for simple filtering and mapping. |

Both engines receive the same `data` map and return the same result types. Compiled programs are cached per expression string — repeated evaluation of the same expression has near-zero overhead.

**CEL example:**
```yaml
filter: 'data.input == "beszel" && data.payload.contains("error")'
```

**Expr example:**
```yaml
filter: 'data.input == "beszel" && contains(data.payload, "error")'
```

### Parser Reference

The `parser` field on each input determines how the raw request body is parsed before expression evaluation. Parsed fields are available as `data.<field>` in expressions.

| Parser | Value | Applicable Content-Type | Output | Description |
|--------|-------|------------------------|--------|-------------|
| JSON | `JSON` | `application/json` | Fields from JSON object | Parses the body as a JSON object. Top-level keys become `data.<key>`. Nested objects are accessible via dot notation in CEL (`data.host.name`). |
| Form | `FORM` | `application/x-www-form-urlencoded` | Form fields | Parses URL-encoded form data. Each key becomes `data.<key>`. Multi-value keys use the first value. |
| XML | `XML` | `application/xml`, `text/xml` | XML element values | Parses XML and flattens element text content into `data.<element>`. Attributes are not parsed. |
| Logfmt | `LOGFMT` | `text/plain` | Logfmt key-value pairs | Parses [logfmt](https://brandur.org/logfmt) format (`key=value key2="value2"`). Each key becomes `data.<key>`. |
| Regex | `REGEX` | `text/plain` | Named capture groups | Applies the `pattern` regex to the raw body. Named capture groups (`(?P<name>...)`) become `data.<name>`. Requires `pattern` to be set. |

If parsing fails (malformed body), the message is still accepted but `data` contains only the base fields (`id`, `input`, `payload`, `createdAt`).

### Hot-Reload

Config hot-reload watches the config file for changes and applies updates without restarting the server.

| Config Section | Hot-Reloadable | Notes |
|----------------|---------------|-------|
| `inputs[].rules` | Yes | Rule changes take effect on the next message processed |
| `outputs[]` | Yes | Output URL, template, retry settings updated immediately |
| `inputs[]` (new/removed) | No | Adding or removing inputs requires a restart |
| `server` | No | Port and timeout changes require a restart |
| `log` | No | Log level/format changes require a restart |
| `storage` | No | Storage path changes require a restart |
| `queue` | No | Queue path/workerCount changes require a restart |
| `worker` | No | Worker defaults require a restart |

### Validation Rules

`validateConfig` checks the following at startup (and on hot-reload):

- Each input must have a non-empty `id` and `type`
- Each input `engine` must be `CEL` or `EXPR`
- Each output must have a non-empty `id` and `type`
- Each output `engine` must be `CEL` or `EXPR`
- Output IDs referenced in `outputIds` must exist in the `outputs` list
- `storage.path` must be non-empty when `storage.type` is `SQLITE`
- `queue.path` must be non-empty when `queue.type` is `FILE`

## API

### Ingest Message

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
Header: `Location: /inputs/{inputId}/messages/{messageId}`

### Get Message

Retrieve a message by ID. The `inputId` is used only for authentication (Bearer token lookup); message lookup is by `messageId` alone.

```
GET /inputs/{inputId}/messages/{messageId}
Authorization: Bearer <secret>
```

Response `200 OK`:
```json
{
  "id": "01JXXXXXXXXXXXXXXXXXXXXXX",
  "version": 1,
  "input": "beszel",
  "payload": {"host": "server1", "status": "down"},
  "createdAt": "2026-03-24T12:00:00Z",
  "status": "PENDING",
  "retryCount": 0
}
```

**`status` values:**

| Value | Description |
|-------|-------------|
| `PENDING` | Queued, not yet processed |
| `DELIVERED` | Successfully delivered to all matched outputs |
| `FAILED` | All retry attempts exhausted without successful delivery |

Note: `lastAttemptAt` is omitted from the response when no delivery has been attempted yet.

**Error responses (RFC 7807):**

| Status | Condition |
|--------|-----------|
| `401 Unauthorized` | Missing or invalid Bearer token |
| `404 Not Found` | Message ID does not exist |

### WebSocket Inbound

```
GET /inputs/{inputId}/messages/ws
Authorization: Bearer <secret>
```

Send JSON messages over the connection; handled identically to HTTP POST.

### TCP Inbound

Connect to the configured `address` and send newline-delimited (or custom `delimiter`) messages. No token auth — secure via network policy.

### Health Check

```
GET /healthz
→ 200 OK
```

### API Documentation

```
GET /docs          → Redoc HTML UI
GET /docs/openapi  → OpenAPI spec (JSON)
GET /docs/asyncapi → AsyncAPI spec (JSON)
```

All HTTP responses include an `X-API-Version` header.

## Architecture

Hexagonal Architecture (Ports & Adapters). Dependencies always flow inward toward the domain.

```
domain (0 deps)
  ↑
application/port/{input,output}  ← interface definitions
  ↑
application/service              ← business logic
  ↑
adapter/{input,output}           ← external world
  ↑
cmd/server/main.go               ← DI assembly, cobra CLI
```

| Path | Role |
|------|------|
| `internal/domain/` | Entities (`Message`, `Output`), enums (`MessageStatus`, `OutputType`), sentinel errors |
| `internal/application/port/input/` | `ReceiveMessageUseCase`, `GetMessageUseCase` interfaces |
| `internal/application/port/output/` | `MessageRepository`, `MessageQueue`, `OutputSender`, `OutputRegistry`, `RuleConfigReader` interfaces |
| `internal/application/service/` | `MessageService` (Receive, GetByID), `RelayWorker` (Start) |
| `internal/config/` | Viper-based YAML loader, `InMemoryRuleConfigReader`, hot-reload (`Watch`) |
| `internal/adapter/input/http/` | chi router, RFC 7807 errors, `X-API-Version` middleware |
| `internal/adapter/input/websocket/` | gorilla/websocket inbound handler |
| `internal/adapter/output/sqlite/` | sqlc-based SQLite repository |
| `internal/adapter/output/filequeue/` | File-based at-least-once queue |
| `internal/adapter/output/webhook/` | HTTP Webhook sender |
| `cmd/server/` | cobra `start` command, full DI assembly |
| `test/e2e/` | End-to-end flow tests |

## Release

Push a version tag to trigger GitHub Actions — it builds all platform binaries and creates a GitHub Release automatically.

```bash
git tag 1.0.0
git push origin 1.0.0
```

Supported platforms: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, `windows/arm64`

Each release includes SHA256 checksums in `checksums.txt`.

## Development

```bash
# Full test suite (with race detector)
go test -race ./... -timeout 60s

# Static analysis
go vet ./...

# Regenerate sqlc code (after SQL changes)
cd internal/adapter/output/sqlite && sqlc generate

# Build for current platform
make build

# Cross-compile all platforms
make build-all
```

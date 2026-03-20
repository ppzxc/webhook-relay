# webhook-relay

모니터링 알람을 수신하여 외부 채널로 전달하는 경량 웹훅 릴레이 허브.

```
beszel / dozzle  →  webhook-relay  →  Slack / Discord / 커스텀 Webhook
```

## 주요 기능

- **다중 소스 수신** — HTTP REST + WebSocket 인바운드
- **YAML 템플릿 변환** — Go `text/template` 기반 페이로드 변환
- **at-least-once 전달** — 파일 큐 기반, 재시작 후에도 메시지 유실 없음
- **지수 백오프 재시도** — 채널별 `retryCount` / `retryDelayMs` 설정
- **설정 핫리로드** — 재시작 없이 channels / routes 변경 반영
- **Bearer token 인증** — 소스별 독립 시크릿

## 빠른 시작

### 사전 요구사항

- Go 1.25+
- GCC (go-sqlite3 CGO 빌드 필요)

```bash
# 빌드
CGO_ENABLED=1 go build -o webhook-relay ./cmd/server/

# 설정 파일 준비
cp internal/config/config.example.yaml config.yaml
# config.yaml 편집 후

# 서버 시작
./webhook-relay start --config config.yaml
```

## 설정

`config.yaml` 예시:

```yaml
server:
  port: 8080

log:
  level: info    # debug | info | warn | error
  format: json   # json | text

sources:
  - id: beszel
    type: BESZEL
    secret: "your-secret"

channels:
  - id: ops-webhook
    type: WEBHOOK
    url: "https://hooks.example.com/xyz"
    template: '{"text": "{{ .Source }}: {{ .Payload }}"}'
    retryCount: 3
    retryDelayMs: 1000

routes:
  - sourceId: beszel
    channelIds: [ops-webhook]

storage:
  type: SQLITE
  path: "./data/webhook-relay.db"

queue:
  type: FILE
  path: "./data/queue"
  workerCount: 2
```

### 템플릿 변수

| 변수 | 설명 |
|------|------|
| `{{ .ID }}` | 알람 ULID |
| `{{ .Source }}` | 소스 타입 (`BESZEL`, `DOZZLE` 등) |
| `{{ .Payload }}` | 원본 JSON 페이로드 (문자열) |
| `{{ .CreatedAt }}` | 수신 시각 (`time.Time`) |

## API

### 알람 수신

```
POST /sources/{sourceId}/alerts
Authorization: Bearer <secret>
Content-Type: application/json

{"host": "server1", "status": "down"}
```

응답 `201 Created`:
```json
{"id": "01J...", "status": "PENDING"}
```

### WebSocket 수신

```
GET /sources/{sourceId}/alerts/ws
Authorization: Bearer <secret>
```

연결 후 JSON 메시지 전송 시 HTTP POST와 동일하게 처리.

### 헬스체크

```
GET /healthz
→ 200 OK
```

모든 응답에 `X-API-Version` 헤더가 포함된다.

## 아키텍처

헥사고날 아키텍처(Ports & Adapters). 의존성은 항상 안쪽(domain)으로만 흐른다.

```
[ HTTP / WebSocket ]
        ↓
  application/service
        ↓
[ SQLite repo ]  [ File queue ]  [ Webhook sender ]
```

## 개발

```bash
# 전체 테스트 (race detector)
go test -race ./... -timeout 60s

# 정적 분석
go vet ./...

# sqlc 재생성 (SQL 변경 시)
cd internal/adapter/output/sqlite && sqlc generate
```

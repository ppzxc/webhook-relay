# webhook-relay Design Spec

**Date:** 2026-03-20
**Status:** Approved

---

## 1. 개요

beszel, dozzle 등 모니터링 앱에서 발생하는 알람을 수신하여, 변환 후 외부 웹훅 또는 기타 채널로 전달하는 허브 앱.

**핵심 목표:**
- 다중 인바운드 수신 (HTTP 웹훅, WebSocket — 인바운드 방향)
- 다중 아웃바운드 전달 (웹훅, 미래: Slack, Discord 등)
- 소스 → 채널 매핑 기반 라우팅 (YAML 설정, 핫리로드)
- YAML 템플릿 기반 페이로드 변환 (재컴파일 불필요)
- SQLite 영구 저장 (어댑터 격리, 추후 교체 가능)
- 파일 큐 at-least-once 전달 (추후 Redis/AMQP 교체 가능)

---

## 2. 아키텍처

**Hexagonal Architecture (Ports & Adapters)**

```
cmd/
└── server/
    └── main.go                        # Cobra root, DI 조립 전용

internal/
├── domain/                            # 순수 엔티티, 열거형 (의존성 0)
│   ├── alert.go
│   ├── alert_status.go
│   ├── channel.go
│   ├── channel_type.go
│   ├── route.go
│   ├── source_type.go
│   └── errors.go
│
├── application/                       # 헥사곤 핵심
│   ├── port/
│   │   ├── input/
│   │   │   └── receive_alert.go       # ReceiveAlertUseCase 인터페이스
│   │   └── output/
│   │       ├── alert_repository.go    # AlertRepository 인터페이스
│   │       ├── alert_sender.go        # AlertSender 인터페이스
│   │       ├── alert_queue.go         # AlertQueue 인터페이스 (at-least-once)
│   │       ├── route_config_reader.go # RouteConfigReader 인터페이스
│   │       └── sender_registry.go     # SenderRegistry 인터페이스
│   └── service/
│       ├── alert_service.go           # ReceiveAlertUseCase 구현
│       └── delivery_worker.go         # DeliveryWorker (application 레이어)
│
└── adapter/
    ├── input/
    │   ├── http/                      # chi 기반 HTTP 웹훅 수신 (인바운드)
    │   └── websocket/                 # gorilla/websocket 수신 (인바운드)
    └── output/
        ├── sqlite/                    # sqlc + mattn/go-sqlite3
        ├── webhook/                   # 범용 HTTP 아웃바운드
        └── filequeue/                 # 파일 기반 큐 (at-least-once)

config/
└── config.go                          # Viper 파서, WatchConfig 핫리로드
```

**의존성 방향 (불변):**
```
adapter → application/port ← application/service
                                      ↓
                           application/port/output
                                      ↑
                                adapter/output
```

- `domain`은 아무것도 import하지 않는다
- `application`은 `domain`만 import한다
- `adapter`는 `application/port`만 import한다
- DI 조립은 `cmd/server/main.go`에서만 수행한다
- 각 어댑터는 컴파일 타임 인터페이스 만족 검증: `var _ port.AlertRepository = (*sqlite.Repository)(nil)`

---

## 3. 도메인 모델

### Alert

```go
type Alert struct {
    ID            string      `json:"id"`
    Version       int         `json:"version"`       // 페이로드 스키마 버전 (마이그레이션 대비)
    Source        SourceType  `json:"source"`
    Payload       RawPayload  `json:"payload"`
    CreatedAt     time.Time   `json:"createdAt"`
    Status        AlertStatus `json:"status"`
    RetryCount    int         `json:"retryCount"`    // 전달 재시도 횟수 (프로세스 재시작 후에도 유지)
    LastAttemptAt *time.Time  `json:"lastAttemptAt"` // 마지막 전달 시도 시각
}

type RawPayload []byte  // MarshalJSON/UnmarshalJSON 구현 (base64 없이 JSON 안전)
```

### 열거형 (string 기반, 직렬화 안전)

```go
type AlertStatus string
const (
    AlertStatusPending   AlertStatus = "PENDING"
    AlertStatusDelivered AlertStatus = "DELIVERED"
    AlertStatusFailed    AlertStatus = "FAILED"
)

// 상태 전이 규칙:
// PENDING → DELIVERED (전달 성공)
// PENDING → FAILED    (재시도 소진)
// FAILED  → PENDING   (수동 재큐, PATCH API)

type ChannelType string
const (
    ChannelTypeWebhook  ChannelType = "WEBHOOK"
    ChannelTypeSlack    ChannelType = "SLACK"
    ChannelTypeDiscord  ChannelType = "DISCORD"
)

type SourceType string
const (
    SourceTypeBeszel  SourceType = "BESZEL"
    SourceTypeDozzle  SourceType = "DOZZLE"
    SourceTypeGeneric SourceType = "GENERIC"
)
```

모든 열거형은 `IsValid()` 메서드 구현.

### Channel

```go
type Channel struct {
    ID           string
    Type         ChannelType
    URL          string
    Template     map[string]string   // key -> CEL/Expr 표현식 (YAML에서 로드)
    Secret       string
    RetryCount   int      // 기본값: 3
    RetryDelayMs int      // 기본값: 1000
}
```

### Route

```go
type Route struct {
    SourceID   string
    ChannelIDs []string
}
```

### 템플릿 렌더링 (RelayWorker)

`Output.Template`은 `map[string]string`으로 각 value는 CEL/Expr 표현식이다.
`RelayWorker.buildPayload(engine, template, data)`에서 각 표현식을 평가해 JSON 페이로드를 생성한다.
- 템플릿이 비어 있으면 `data["payload"]` 원문을 그대로 전달
- `data` 컨텍스트: `data.id`, `data.input`, `data.payload`, `data.createdAt`, mapping으로 추가된 필드

예시:
```yaml
template:
  text: 'data.input + ": " + data.payload'
```

---

## 4. 포트 인터페이스

```go
// application/port/input/receive_alert.go
type ReceiveAlertUseCase interface {
    Receive(ctx context.Context, sourceID string, payload []byte) error
}

// application/port/output/alert_repository.go
type AlertRepository interface {
    Save(ctx context.Context, alert domain.Alert) error
    UpdateDeliveryState(ctx context.Context, id string, status domain.AlertStatus, retryCount int, lastAttemptAt time.Time) error
    FindByID(ctx context.Context, id string) (domain.Alert, error)
    FindBySource(ctx context.Context, sourceID string, limit, offset int) ([]domain.Alert, error)
}

// application/port/output/alert_sender.go
type AlertSender interface {
    Send(ctx context.Context, channel domain.Channel, alert domain.Alert) error
}

// application/port/output/alert_queue.go
// at-least-once: Dequeue 후 반드시 AckFunc 또는 NackFunc 호출
type AlertQueue interface {
    Enqueue(ctx context.Context, alert domain.Alert) error
    Dequeue(ctx context.Context) (domain.Alert, AckFunc, NackFunc, error)
}
type AckFunc  func() error  // 전달 성공 시 호출 (큐에서 영구 삭제)
type NackFunc func() error  // 전달 실패 시 호출 (큐에 메시지 반환)

// application/port/output/route_config_reader.go
type RouteConfigReader interface {
    GetChannels(ctx context.Context, sourceID string) ([]domain.Channel, error)
}

// application/port/output/sender_registry.go
// ChannelType별 AlertSender 구현체 반환 (DI에서 등록, application은 타입 스위치 없음)
type SenderRegistry interface {
    Get(channelType domain.ChannelType) (AlertSender, error)
}
```

---

## 5. API 설계

**버저닝:** URL 경로 버저닝 금지 (`/v1/...` 불허), 헤더 버저닝 사용
```
X-API-Version: 2026-03-20
```

**인증:** `Authorization: Bearer <secret>` (소스별 설정)

**엔드포인트:**

| Method | URL | 설명 |
|--------|-----|------|
| POST | `/sources/{sourceId}/alerts` | 알람 수신 |
| GET | `/sources/{sourceId}/alerts` | 알람 목록 |
| GET | `/sources/{sourceId}/alerts/{alertId}` | 알람 단건 |
| PATCH | `/sources/{sourceId}/alerts/{alertId}` | 상태 변경 (FAILED→PENDING 재큐) |
| GET | `/sources/{sourceId}/alerts/ws` | WebSocket 수신 (인바운드) |
| GET | `/sources` | 소스 목록 |
| GET | `/sources/{sourceId}` | 소스 단건 |
| GET | `/channels` | 채널 목록 |
| GET | `/channels/{channelId}` | 채널 단건 |
| GET | `/healthz` | 헬스체크 |

> **라우터 등록 순서:** `/alerts/ws` 리터럴 경로를 `/alerts/{alertId}` 와일드카드보다 먼저 등록.

**WebSocket 방향:** 인바운드 전용. 모니터링 앱이 WebSocket으로 연결해 알람을 푸시하면, 동일한 `ReceiveAlertUseCase.Receive()`를 호출한다. (아웃바운드 스트리밍 아님)

**PATCH 허용 전이:**
```json
{ "status": "PENDING" }   // FAILED → PENDING (수동 재큐)
```
다른 전이는 422 반환.

**201 Created 응답:**
```json
{
  "id": "01J...",
  "sourceId": "beszel",
  "status": "PENDING",
  "createdAt": "2026-03-20T12:00:00Z"
}
```
`Location: /sources/beszel/alerts/01J...`

**에러 응답 (RFC 7807):**
```json
{
  "type": "/errors/unauthorized",
  "title": "Unauthorized",
  "status": 401,
  "detail": "Invalid or missing token for source: beszel",
  "traceId": "abc-123"
}
```
`Content-Type: application/problem+json`

---

## 6. 설정 파일

Cobra + Viper 사용. `viper.WatchConfig()` + `viper.OnConfigChange()`로 핫리로드.

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
  level: info      # debug, info, warn, error
  format: json     # json, text

sources:
  - id: beszel
    type: BESZEL
    secret: "your-secret-token"
  - id: dozzle
    type: DOZZLE
    secret: "your-secret-token"

channels:
  - id: ops-webhook
    type: WEBHOOK
    url: "https://hooks.example.com/xyz"
    template: |
      {"text": "{{ .Source }}: {{ .Payload }}"}
    retryCount: 3
    retryDelayMs: 1000
    skipTLSVerify: false   # 내부망 아웃바운드용

routes:
  - sourceId: beszel
    channelIds: [ops-webhook]
  - sourceId: dozzle
    channelIds: [ops-webhook]

storage:
  type: SQLITE
  path: "./data/webhook-relay.db"

queue:
  type: FILE
  path: "./data/queue"
  workerCount: 2     # DeliveryWorker 고루틴 수

```

**우선순위:** CLI 플래그 > 환경변수 > config.yaml > 기본값

**핫리로드 동작:**
- `channels`, `routes` 변경: 파싱 및 템플릿 검증 후 유효한 경우에만 즉시 반영. 유효하지 않으면 기존 설정 유지 + 에러 로그.
- `server.port`, `storage`, `queue` 변경: 재시작 필요, 로그로 안내.

---

## 7. 데이터 플로우

```
[모니터링 앱]
     │ POST /sources/beszel/alerts  (또는 WebSocket)
     ▼
[adapter/input/http 또는 adapter/input/websocket]
  1. Secret Token 검증
  2. ReceiveAlertUseCase.Receive() 호출
     │
     ▼
[application/service/AlertService]
  3. Alert 엔티티 생성 (ID 채번, status=PENDING, version=1)
  4. AlertRepository.Save()         ← SQLite 영구 저장
  5. AlertQueue.Enqueue()           ← 파일 큐 적재 (at-least-once)
  6. 즉시 201 반환                  ← 비동기 전달 분리
     │
     ▼
[application/service/DeliveryWorker] ← workerCount 개수의 고루틴
  7. AlertQueue.Dequeue()           ← AckFunc, NackFunc 수령
  8. RouteConfigReader.GetChannels() → []Channel
  9. SenderRegistry.Get(channel.Type) → AlertSender
 10. domain.RenderTemplate(channel.Template, alert) → payload
 11. AlertSender.Send()             ← 지수 백오프 재시도 (channel.RetryCount)
 12a. 성공: AckFunc() → 에러 시 경고 로그 후 DELIVERED로 계속 진행 (at-least-once 보장, 중복 전달 허용)
     UpdateDeliveryState(DELIVERED)
 12b. 실패: NackFunc() + UpdateDeliveryState(FAILED, retryCount+1)
```

- 수신과 전달 분리: 아웃바운드 장애가 인바운드 응답 지연에 영향 없음
- `DeliveryWorker`는 `context.Done()`으로 graceful shutdown
- 재시도 소진 후 `FAILED` → `PATCH` API로 수동 `PENDING` 재큐 가능
- 프로세스 재시작 후 `RetryCount`는 DB에서 복원됨

---

## 8. 에러 처리

```go
// domain/errors.go — sentinel errors
var (
    ErrSourceNotFound  = errors.New("source not found")
    ErrInvalidToken    = errors.New("invalid token")
    ErrAlertNotFound   = errors.New("alert not found")
    ErrInvalidTransition = errors.New("invalid status transition")
    ErrSenderNotFound  = errors.New("sender not registered for channel type")
)

// 에러 체인 보존 (%w)
return fmt.Errorf("save alert: %w", err)

// HTTP 미들웨어 매핑
errors.Is(err, domain.ErrInvalidToken)       → 401
errors.Is(err, domain.ErrSourceNotFound)     → 404
errors.Is(err, domain.ErrAlertNotFound)      → 404
errors.Is(err, domain.ErrInvalidTransition)  → 422
default                                       → 500
```

모든 유효성 실패는 한 번에 반환 (RFC 7807).
스택 트레이스, 내부 경로, DB 에러 노출 금지.

---

## 9. 테스트 전략

| 레이어 | 파일 | 방식 |
|--------|------|------|
| `domain/` | `*_test.go` | 순수 단위 테스트, 의존성 0 |
| `application/service/alert_service_test.go` | — | 포트 인터페이스 수동 Mock (function field) |
| `application/service/delivery_worker_test.go` | — | 수동 Mock: AlertQueue, AlertRepository, AlertSender, RouteConfigReader, SenderRegistry |
| `adapter/input/http/` | — | `httptest` 패키지 |
| `adapter/output/sqlite/` | — | 실제 in-memory SQLite (`?mode=memory`) |
| `adapter/output/filequeue/` | — | 실제 파일시스템 (`t.TempDir()`) |
| E2E | `e2e/` | `httptest.Server` + 실제 DI 조립 |

- mock 프레임워크 미사용
- DB mock 미사용
- table-driven 테스트 + `t.Run()`
- 각 어댑터 컴파일 타임 검증: `var _ port.AlertRepository = (*sqlite.Repository)(nil)`

---

## 10. 기술 스택

| 역할 | 라이브러리 |
|------|-----------|
| CLI | `cobra` |
| 설정 | `viper` |
| HTTP 라우터 | `chi` |
| WebSocket | `gorilla/websocket` |
| SQLite | `sqlc` + `mattn/go-sqlite3` |
| 로깅 | `slog` (Go 표준) |

---

## 11. 확장 포인트

| 현재 구현 | 교체/확장 방법 |
|-----------|--------------|
| SQLite | PostgreSQL, MySQL — `AlertRepository` 어댑터 교체 |
| 파일 큐 | Redis, RabbitMQ/AMQP — `AlertQueue` 어댑터 교체 |
| HTTP + WebSocket (인바운드) | MQTT, gRPC — `input` 어댑터 추가 |
| Webhook (아웃바운드) | Slack, Discord, Telegram — `output` 어댑터 추가 + `SenderRegistry` 등록 |
| 소스→채널 단순 매핑 | 조건부 라우팅 (심각도, 키워드) — `RouteConfigReader` 구현 교체 |

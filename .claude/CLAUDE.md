# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
# 빌드 (CGO 필수 — go-sqlite3)
CGO_ENABLED=1 go build -o relaybox ./cmd/server/

# 전체 테스트 (race detector 포함)
go test -race ./... -timeout 60s

# 특정 패키지 테스트
go test -race ./internal/adapter/output/sqlite/...
go test -race ./test/e2e/...

# 특정 테스트 함수 하나만 실행
go test -race -run TestRelayWorker_DeliverSuccess ./internal/application/service/

# 정적 분석
go vet ./...

# sqlc 코드 재생성 (query.sql / schema.sql 변경 시)
cd internal/adapter/output/sqlite && sqlc generate
```

> `CGO_ENABLED=1` 없이 빌드하면 `go-sqlite3`가 빌드 실패한다. `.claude/settings.local.json`의 `env`에 이미 설정되어 있다.

## Architecture

헥사고날 아키텍처(Ports & Adapters). 의존성 방향은 항상 안쪽으로만 흐른다.

```
domain (0 deps)
  ↑
application/port/{input,output}  ← 인터페이스 정의
  ↑
application/service              ← 비즈니스 로직
  ↑
adapter/{input,output}           ← 외부 세계와 연결
  ↑
cmd/server/main.go               ← DI 조립, cobra CLI
```

### 레이어별 역할

| 경로 | 역할 |
|------|------|
| `internal/domain/` | 엔티티(`Message`, `Output`), 열거형(`InputType`, `MessageStatus`, `OutputType`), 센티넬 에러 |
| `internal/application/port/input/` | `ReceiveMessageUseCase` 인터페이스 |
| `internal/application/port/output/` | `MessageRepository`, `MessageQueue`, `OutputSender`, `OutputRegistry`, `RuleConfigReader` 인터페이스 |
| `internal/application/service/` | `MessageService`(Receive), `RelayWorker`(Start) |
| `internal/config/` | Viper 기반 YAML 로더, `InMemoryRuleConfigReader`, hot-reload(`Watch`) |
| `internal/adapter/input/http/` | chi 라우터, RFC 7807 에러, `X-API-Version` 헤더 미들웨어 |
| `internal/adapter/input/websocket/` | gorilla/websocket 인바운드 핸들러 |
| `internal/adapter/output/sqlite/` | sqlc 기반 SQLite 저장소 |
| `internal/adapter/output/filequeue/` | 파일 기반 at-least-once 큐 |
| `internal/adapter/output/webhook/` | HTTP Webhook 송신 |
| `cmd/server/` | cobra `start` 커맨드, 전체 DI 조립 |
| `test/e2e/` | 전체 흐름 E2E 테스트 |

## Key Design Decisions

### 열거형
모든 열거형은 `type X string` + 대문자 상수(`"BESZEL"`, `"PENDING"` 등). 별도 `MarshalJSON` 불필요.

### 라우팅 키
`InMemoryRuleConfigReader`는 rules를 **input type**(e.g. `"BESZEL"`)으로 인덱싱한다. `Update()`에서 `inputID → inputType` 변환을 수행하므로, `GetOutputs`는 반드시 `string(msg.Input)`(타입 값)를 넘겨야 한다.

### 큐 at-least-once 보장
파일 큐는 `Dequeue` 시 `.json` → `.json.processing` 으로 rename. `Ack`는 `.processing` 삭제, `Nack`는 원래 이름으로 rename 복구.

### AckFunc / NackFunc
`MessageQueue.Dequeue`는 `(domain.Message, AckFunc, NackFunc, error)`를 반환한다. `AckFunc`와 `NackFunc`는 `output` 패키지에 정의된 함수 타입.

### HTTP API
- URL versioning 없이 `X-API-Version` 응답 헤더 사용
- 에러 포맷: RFC 7807 (`application/problem+json`)
- `POST /inputs/{inputId}/messages` → 201 + `Location: /inputs/{inputId}/messages/{messageId}`
- Bearer token 인증 (`Authorization: Bearer <secret>`)
- WebSocket: `GET /inputs/{inputId}/messages/ws`

### sqlc
`internal/adapter/output/sqlite/db/` 는 자동 생성 코드. `query.sql` / `schema.sql` 수정 후 `sqlc generate` 재실행. 직접 편집 금지.

### DI 조립
`cmd/server/main.go`의 `runServer()` 함수가 전체 어댑터를 조립한다. `configInputResolver`는 config 파일 기반으로 `InputResolver` 인터페이스를 구현한다.

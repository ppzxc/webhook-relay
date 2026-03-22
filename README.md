# relaybox

범용 릴레이 허브: 어떤 인바운드 프로토콜/포맷도 수신하고, CEL/Expr 표현식 기반 필터·변환·라우팅 규칙을 통해 아웃바운드 채널로 전달한다.

```
어떤 인바운드 (HTTP REST / WebSocket / TCP / ...)
        ↓
  파서 파이프라인 (JSON / Form / XML / Logfmt / Regex)
        ↓
  CEL / Expr 표현식 필터 + 변환 + 라우팅
        ↓
어떤 아웃바운드 (Webhook / Slack / Discord / ...)
```

## 주요 기능

- **멀티 프로토콜 인바운드** — HTTP REST + WebSocket + TCP
- **파서 파이프라인** — 입력별로 JSON, Form, XML, Logfmt, Regex 지원
- **표현식 기반 라우팅** — 규칙별 CEL/Expr 필터, 매핑, 라우팅 조건
- **at-least-once 전달** — 파일 큐 기반, 재시작 시에도 메시지 보존
- **지수 백오프 재시도** — 채널별 `retryCount` / `retryDelayMs` 설정
- **설정 핫리로드** — 재시작 없이 아웃풋 / 규칙 변경 가능
- **Bearer 토큰 인증** — 입력별 독립 시크릿

## 빠른 시작

### 사전 요구 사항

- Go 1.25+
- GCC (go-sqlite3 CGO 빌드에 필요)

```bash
# 빌드
CGO_ENABLED=1 go build -o relaybox ./cmd/server/

# 설정 준비
cp docs/config.example.yaml config.yaml
# config.yaml 수정 후:

# 서버 시작
./relaybox start --config config.yaml
```

## 설정

`config.yaml` 예시:

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
    secret: ""        # TCP 입력은 시크릿 미사용

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
    engine: cel           # 규칙별 엔진 오버라이드
    filter: 'data.input == "BESZEL"'
    mapping:
      severity: '"HIGH"'
    routing:
      - condition: 'data.severity == "HIGH"'
        outputIds: [ops-webhook]
  - inputId: tcp-input
    outputIds: [ops-webhook]  # 단순: 필터/라우팅 없이 전체 전송

storage:
  type: SQLITE
  path: "./data/relaybox.db"

queue:
  type: FILE
  path: "./data/queue"
  workerCount: 2
```

### 표현식 변수

모든 표현식(필터, 매핑, 라우팅, 템플릿)은 동일한 `data` 컨텍스트를 공유한다:

| 변수 | 설명 |
|------|------|
| `data.id` | 메시지 ULID |
| `data.input` | 입력 타입 (`BESZEL`, `DOZZLE`, `GENERIC` 등) |
| `data.payload` | 원본 페이로드 문자열 |
| `data.createdAt` | 수신 타임스탬프 (RFC3339) |
| `data.<field>` | `mapping` 표현식으로 추가된 필드 |

**필터** — 불리언 표현식; `false`이면 메시지 드롭:
```yaml
filter: 'data.input == "BESZEL"'
```

**매핑** — 계산된 필드로 `data` 보강:
```yaml
mapping:
  severity: '"HIGH"'
  label: 'data.input + "-alert"'
```

**라우팅** — 조건부 아웃풋 선택 (매핑 이후 평가):
```yaml
routing:
  - condition: 'data.severity == "HIGH"'
    outputIds: [ops-webhook]
```

**템플릿** — 아웃풋 필드를 표현식으로 렌더링:
```yaml
template:
  text: 'data.input + ": " + data.payload'
```

## API

### 메시지 수신

```
POST /inputs/{inputId}/messages
Authorization: Bearer <secret>
Content-Type: application/json

{"host": "server1", "status": "down"}
```

응답 `201 Created`:
```json
{"id": "01J...", "status": "PENDING"}
```

### WebSocket 인바운드

```
GET /inputs/{inputId}/messages/ws
Authorization: Bearer <secret>
```

연결 후 JSON 메시지를 전송하면 HTTP POST와 동일하게 처리된다.

### TCP 인바운드

설정한 `address`로 연결 후 개행(또는 커스텀 `delimiter`) 구분 메시지를 전송한다. 토큰 인증 없음 — 네트워크 정책으로 보안 적용.

### 헬스 체크

```
GET /healthz
→ 200 OK
```

모든 HTTP 응답에는 `X-API-Version` 헤더가 포함된다.

## 아키텍처

헥사고날 아키텍처(Ports & Adapters). 의존성 방향은 항상 도메인을 향해 안쪽으로만 흐른다.

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

## 개발

```bash
# 전체 테스트 (race detector 포함)
go test -race ./... -timeout 60s

# 정적 분석
go vet ./...

# sqlc 코드 재생성 (SQL 변경 후)
cd internal/adapter/output/sqlite && sqlc generate
```

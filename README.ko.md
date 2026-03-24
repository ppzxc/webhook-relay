[English](README.md) | **한국어**

# relaybox

범용 릴레이 허브: 어떤 인바운드 프로토콜/포맷도 수신하고, CEL/Expr 표현식 기반 필터·변환·라우팅 규칙을 통해 아웃바운드 채널로 전달한다.

```
어떤 인바운드 (HTTP REST / WebSocket / TCP / ...)
        ↓
  파서 파이프라인 (JSON / Form / XML / Logfmt / Regex)
        ↓
  CEL / Expr 표현식 필터 + 변환 + 라우팅
        ↓
어떤 아웃바운드 (Webhook / ...)
```

## 주요 기능

- **멀티 프로토콜 인바운드** — HTTP REST, WebSocket, TCP
- **파서 파이프라인** — 입력별로 JSON, Form, XML, Logfmt, Regex(커스텀 패턴) 지원
- **표현식 기반 라우팅** — 입력별 CEL/Expr 필터, 매핑, 라우팅 규칙
- **at-least-once 전달** — 파일 큐 기반, 재시작 시에도 메시지 보존
- **지수 백오프 재시도** — 아웃풋별 `retryCount` / `retryDelayMs` 설정
- **설정 핫리로드** — 재시작 없이 아웃풋/규칙 변경 가능
- **Bearer 토큰 인증** — 입력별 독립 시크릿
- **Dot-notation 템플릿** — `parent.child` 키로 중첩 JSON 출력 생성

## 빠른 시작

### 사전 요구 사항

- Go 1.25+

### 빌드

```bash
# 클론 후 빌드
go build -o relaybox ./cmd/server/

# 설정 준비
cp docs/config.example.yaml config.yaml
# config.yaml 수정 후:

# 서버 시작
./relaybox start --config config.yaml
```

### Makefile

```bash
# 현재 플랫폼 빌드
make build

# 전체 플랫폼 크로스 컴파일 (dist/ 디렉토리에 출력)
make build-all

# 테스트 실행
make test

# 릴리스 빌드 (clean + build-all + checksums)
make release VERSION=1.0.0
```

## 설정

### 빠른 예제

`config.yaml` 예시 (전체 레퍼런스는 `docs/config.example.yaml` 참고):

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
    engine: CEL          # 필수 — CEL 또는 EXPR
    parser: JSON         # JSON, FORM, XML, LOGFMT, REGEX
    secret: "change-me"
    rules:
      # Rule 1: 조건부 라우팅
      - filter: 'data.severity == "HIGH"'
        mapping:
          level: '"CRITICAL"'
        routing:
          - condition: 'data.level == "CRITICAL"'
            outputIds: [ops-webhook]
      # Rule 2: 필터 없이 전체 전송
      - outputIds: [notify-bot]

  - id: tcp-input
    engine: CEL
    address: ":9001"
    delimiter: "\n"
    parser: JSON
    rules:
      - outputIds: [ops-webhook]   # 단순: 필터 없이 전체 전송

outputs:
  - id: ops-webhook
    type: WEBHOOK
    engine: CEL          # 필수 — CEL 또는 EXPR
    url: "https://hooks.example.com/xyz"
    template:
      text: 'data.input + ": " + data.payload'
    retryCount: 3
    retryDelayMs: 1000
    skipTLSVerify: false

  # Dot-notation 키는 중첩 JSON을 생성한다
  - id: notify-bot
    type: WEBHOOK
    engine: CEL
    url: "https://example.com/api/v1/bots/1/text"
    secret: "bearer-token"   # Authorization: Bearer <secret> 로 전송
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
  defaultRetryCount: 3      # 아웃풋에 retryCount 없을 때 폴백
  defaultRetryDelay: "1s"   # 폴백 기본 재시도 대기 (Go duration)
  pollBackoff: "500ms"      # 빈 큐 폴링 간격
```

### 설정 레퍼런스

#### `server`

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `port` | int | 아니오 | `8080` | HTTP 서버 리슨 포트 |
| `readTimeout` | duration | 아니오 | `30s` | 요청을 완전히 읽을 때까지의 최대 시간. Go duration 문자열 (예: `10s`, `1m`). |
| `writeTimeout` | duration | 아니오 | `30s` | 응답을 완전히 전송할 때까지의 최대 시간. Go duration 문자열. |

#### `server.tls`

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `enabled` | bool | 아니오 | `false` | TLS 활성화. `true`로 설정 시 `certFile`과 `keyFile`이 필수. |
| `certFile` | string | 아니오 | `""` | PEM 인코딩 TLS 인증서 파일 경로. |
| `keyFile` | string | 아니오 | `""` | PEM 인코딩 TLS 개인키 파일 경로. |

#### `log`

| 필드 | 타입 | 필수 | 기본값 | 허용값 | 설명 |
|------|------|------|--------|--------|------|
| `level` | string | 아니오 | `INFO` | `DEBUG`, `INFO`, `WARN`, `ERROR` | 최소 로그 레벨. `DEBUG`는 모든 레벨 출력, `ERROR`는 에러만 출력. |
| `format` | string | 아니오 | `JSON` | `JSON`, `TEXT` | 로그 출력 포맷. `JSON`은 JSON Lines 형식, `TEXT`는 사람이 읽기 쉬운 key=value 형식. |

#### `inputs[]`

`inputs` 목록의 각 항목은 인바운드 엔드포인트 하나를 정의한다.

| 필드 | 타입 | 필수 | 기본값 | 허용값 | 설명 |
|------|------|------|--------|--------|------|
| `id` | string | **예** | — | 유니크한 문자열 | API URL에 사용되는 식별자 (`/inputs/{id}/messages`). 모든 입력 중 유니크해야 함. 모든 메시지에 `data.input`으로 저장됨. |
| `engine` | string | **예** | — | `CEL`, `EXPR` | 이 입력의 filter/mapping/routing 규칙 평가에 사용할 표현식 엔진. [표현식 엔진 레퍼런스](#표현식-엔진-레퍼런스) 참고. |
| `parser` | string | 아니오 | `JSON` | `JSON`, `FORM`, `XML`, `LOGFMT`, `REGEX` | 표현식 평가 전 원본 요청 바디에 적용할 파서. [파서 레퍼런스](#파서-레퍼런스) 참고. |
| `secret` | string | 아니오 | `""` | 임의 문자열 | `Authorization: Bearer <secret>` 헤더에 필요한 Bearer 토큰. 빈 문자열이면 인증 없이 모든 요청 허용. |
| `address` | string | 아니오 | `""` | 예: `:9001` | TCP 리슨 주소. TCP 입력에만 적용. 설정 시 해당 주소에서 TCP 연결 수신. HTTP/WebSocket 입력에는 사용되지 않음. |
| `delimiter` | string | 아니오 | `"\n"` | 임의 문자열 | TCP 스트림의 레코드 구분자. 이 문자열로 메시지를 분리. TCP 입력에만 적용. |
| `pattern` | string | 아니오 | `""` | RE2 정규식 | `REGEX` 파서용 Named capture group 정규식. `parser: REGEX` 사용 시 필수. 예: `(?P<level>\w+) (?P<msg>.+)`. |
| `rules` | list | 아니오 | `[]` | — | 수신된 메시지마다 순서대로 평가되는 라우팅 규칙 목록. `inputs[].rules[]` 참고. |

#### `inputs[].rules[]`

규칙은 순서대로 평가된다. `filter` 표현식이 `true`를 반환하거나 `filter`가 없으면 규칙이 매칭된다. 매칭된 규칙의 `mapping`이 적용되고 `outputIds` 또는 `routing`으로 아웃풋이 선택된다.

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `filter` | string | 아니오 | `""` (항상 true) | CEL/Expr 불리언 표현식. `false`이면 이 규칙을 건너뜀. 없으면 항상 매칭. |
| `mapping` | map[string]string | 아니오 | `{}` | 키-값 쌍으로, 각 값은 CEL/Expr 표현식. 평가 결과가 `data`에 추가됨. `routing` 평가 전에 적용. 기존 필드 덮어쓰기 가능. |
| `outputIds` | list[string] | 아니오 | `[]` | 이 규칙이 매칭되고 `routing` 조건이 발동되지 않을 때 전송할 아웃풋 ID 목록. 폴백으로 동작. |
| `routing` | list | 아니오 | `[]` | `mapping` 이후 평가되는 조건부 아웃풋 선택. `inputs[].rules[].routing[]` 참고. |

#### `inputs[].rules[].routing[]`

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `condition` | string | 아니오 | `""` (항상 true) | `data`(매핑된 필드 포함)에 대해 평가되는 CEL/Expr 불리언 표현식. `true`이면 `outputIds`로 메시지 전송. |
| `outputIds` | list[string] | **예** | — | `condition`이 true일 때 전송할 아웃풋 ID 목록. |

#### `outputs[]`

각 항목은 아웃바운드 목적지 하나를 정의한다.

| 필드 | 타입 | 필수 | 기본값 | 허용값 | 설명 |
|------|------|------|--------|--------|------|
| `id` | string | **예** | — | 유니크한 문자열 | 규칙의 `outputIds`에서 참조하는 식별자. 모든 아웃풋 중 유니크해야 함. |
| `type` | string | **예** | — | `WEBHOOK` | 아웃풋 타입. 현재 `WEBHOOK`만 지원. |
| `engine` | string | **예** | — | `CEL`, `EXPR` | `template` 표현식 평가에 사용할 표현식 엔진. [표현식 엔진 레퍼런스](#표현식-엔진-레퍼런스) 참고. |
| `url` | string | **예** (WEBHOOK) | — | HTTP/HTTPS URL | 웹훅 전송 목적지 URL. |
| `template` | map[string]string | 아니오 | `{}` | — | 키-값 쌍으로, 각 값은 아웃바운드 JSON 바디에 렌더링되는 CEL/Expr 표현식. Dot-notation 키(예: `content.type`)는 중첩 JSON 객체를 생성. |
| `secret` | string | 아니오 | `""` | 임의 문자열 | 설정 시 아웃바운드 웹훅 요청에 `Authorization: Bearer <secret>` 헤더로 전송. |
| `retryCount` | int | 아니오 | `0` (`worker.defaultRetryCount` 사용) | `0`–N | 첫 실패 후 재시도 횟수. `0`이면 워커 레벨 기본값 사용. 재시도는 `retryDelayMs`를 기준으로 지수 백오프 적용. |
| `retryDelayMs` | int | 아니오 | `0` (`worker.defaultRetryDelay` 사용) | `0`–N | 재시도 간 기본 대기 시간(밀리초). 매 재시도마다 2배씩 증가. `0`이면 워커 레벨 기본값 사용. |
| `timeoutSec` | int | 아니오 | `0` (타임아웃 없음) | `0`–N | HTTP 요청 타임아웃(초). `0`이면 타임아웃 없음. |
| `skipTLSVerify` | bool | 아니오 | `false` | `true`, `false` | HTTPS 목적지의 TLS 인증서 검증 스킵. 개발 환경이나 자체 서명 인증서 사용 시에만 활성화. |

#### `storage`

| 필드 | 타입 | 필수 | 기본값 | 허용값 | 설명 |
|------|------|------|--------|--------|------|
| `type` | string | **예** | — | `SQLITE` | 스토리지 백엔드 타입. 현재 `SQLITE`만 지원. |
| `path` | string | **예** | — | 파일 경로 | SQLite 데이터베이스 파일 경로. 없으면 자동 생성. 예: `./data/relaybox.db`. |

#### `queue`

| 필드 | 타입 | 필수 | 기본값 | 허용값 | 설명 |
|------|------|------|--------|--------|------|
| `type` | string | **예** | — | `FILE` | 큐 백엔드 타입. 현재 `FILE`만 지원. 메시지는 JSON 파일로 저장되어 재시작 시에도 보존됨. |
| `path` | string | **예** | — | 디렉토리 경로 | 큐 메시지 파일을 저장하는 디렉토리. 없으면 자동 생성. 예: `./data/queue`. |
| `workerCount` | int | 아니오 | `1` | `1`–N | 큐를 처리하는 동시 릴레이 워커 고루틴 수. 높을수록 처리량이 증가하지만 CPU/메모리를 더 소비. |

#### `worker`

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `defaultRetryCount` | int | 아니오 | `3` | 아웃풋의 `retryCount`가 `0`이거나 설정되지 않은 경우 사용하는 폴백 재시도 횟수. `0` 이하로 설정하면 재시도 없음. |
| `defaultRetryDelay` | duration | 아니오 | `1s` | 아웃풋의 `retryDelayMs`가 `0`이거나 설정되지 않은 경우 사용하는 폴백 기본 재시도 대기 시간. Go duration 문자열 (예: `500ms`, `2s`). |
| `pollBackoff` | duration | 아니오 | `500ms` | 큐가 비었을 때 다음 폴링까지 대기 시간. 낮을수록 지연이 줄지만 CPU 사용량 증가. Go duration 문자열. |

### 표현식 변수

모든 표현식(필터, 매핑, 라우팅, 템플릿)은 동일한 `data` 컨텍스트를 공유한다:

| 변수 | 타입 | 설명 |
|------|------|------|
| `data.id` | string | 메시지 ULID — 수신 시 자동 할당되는 유니크 식별자 |
| `data.input` | string | 입력 ID 값 (예: `beszel`, `dozzle`) |
| `data.payload` | string | 수신된 원본 페이로드 문자열 |
| `data.createdAt` | string | RFC3339 형식의 수신 타임스탬프 |
| `data.<field>` | any | `mapping` 표현식으로 추가되거나 덮어쓰인 필드 |

**필터** — 불리언 표현식; `false`이면 이 규칙에서 메시지 드롭:
```yaml
filter: 'data.input == "beszel"'
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

**템플릿** — 표현식으로 아웃풋 필드를 렌더링. Dot-notation 키는 중첩 JSON을 생성한다:
```yaml
template:
  text: 'data.input + ": " + data.payload'
  content.type: '"text"'
  content.text: 'data.payload'
```

### 표현식 엔진 레퍼런스

| 엔진 | 값 | 라이브러리 | 설명 |
|------|----|-----------|----|
| CEL | `CEL` | [google/cel-go](https://github.com/google/cel-go) | Google Common Expression Language. 강타입, 시작 시 컴파일. 컴파일 에러를 런타임이 아닌 시작 시 발견하므로 프로덕션에 권장. |
| Expr | `EXPR` | [expr-lang/expr](https://github.com/expr-lang/expr) | 경량 표현식 평가기. 덕타이핑, 간단한 문법. 단순 필터링과 매핑에 적합. |

두 엔진 모두 동일한 `data` 맵을 받고 동일한 결과 타입을 반환한다. 컴파일된 프로그램은 표현식 문자열 단위로 캐싱되므로 동일한 표현식의 반복 평가는 오버헤드가 거의 없다.

**CEL 예제:**
```yaml
filter: 'data.input == "beszel" && data.payload.contains("error")'
```

**Expr 예제:**
```yaml
filter: 'data.input == "beszel" && contains(data.payload, "error")'
```

### 파서 레퍼런스

입력의 `parser` 필드는 표현식 평가 전 원본 요청 바디를 어떻게 파싱할지 결정한다. 파싱된 필드는 표현식에서 `data.<field>`로 접근 가능하다.

| 파서 | 값 | 적용 Content-Type | 출력 | 설명 |
|------|----|--------------------|------|------|
| JSON | `JSON` | `application/json` | JSON 객체 필드 | 바디를 JSON 객체로 파싱. 최상위 키가 `data.<key>`가 됨. 중첩 객체는 CEL에서 dot notation으로 접근 가능 (`data.host.name`). |
| Form | `FORM` | `application/x-www-form-urlencoded` | 폼 필드 | URL 인코딩된 폼 데이터 파싱. 각 키가 `data.<key>`가 됨. 다중값 키는 첫 번째 값 사용. |
| XML | `XML` | `application/xml`, `text/xml` | XML 요소 값 | XML을 파싱하고 요소의 텍스트 콘텐츠를 `data.<element>`로 평탄화. 속성은 파싱되지 않음. |
| Logfmt | `LOGFMT` | `text/plain` | Logfmt 키-값 쌍 | [logfmt](https://brandur.org/logfmt) 형식 (`key=value key2="value2"`) 파싱. 각 키가 `data.<key>`가 됨. |
| Regex | `REGEX` | `text/plain` | Named capture group | 원본 바디에 `pattern` 정규식을 적용. Named capture group(`(?P<name>...)`)이 `data.<name>`이 됨. `pattern` 설정 필수. |

파싱 실패(잘못된 바디) 시에도 메시지는 수신되지만 `data`에는 기본 필드(`id`, `input`, `payload`, `createdAt`)만 포함된다.

### 핫리로드

설정 핫리로드는 설정 파일의 변경을 감지하고 서버 재시작 없이 업데이트를 적용한다.

| 설정 섹션 | 핫리로드 가능 | 비고 |
|-----------|-------------|------|
| `inputs[].rules` | 예 | 다음 메시지 처리 시 변경 적용 |
| `outputs[]` | 예 | URL, 템플릿, 재시도 설정 즉시 업데이트 |
| `inputs[]` (추가/삭제) | 아니오 | 입력 추가/삭제 시 재시작 필요 |
| `server` | 아니오 | 포트 및 타임아웃 변경 시 재시작 필요 |
| `log` | 아니오 | 로그 레벨/포맷 변경 시 재시작 필요 |
| `storage` | 아니오 | 스토리지 경로 변경 시 재시작 필요 |
| `queue` | 아니오 | 큐 경로/workerCount 변경 시 재시작 필요 |
| `worker` | 아니오 | 워커 기본값 변경 시 재시작 필요 |

### 유효성 검사 규칙

`validateConfig`는 시작 시(및 핫리로드 시) 다음을 확인한다:

- 각 입력은 비어 있지 않은 `id`와 `type`을 가져야 함
- 각 입력의 `engine`은 `CEL` 또는 `EXPR`이어야 함
- 각 아웃풋은 비어 있지 않은 `id`와 `type`을 가져야 함
- 각 아웃풋의 `engine`은 `CEL` 또는 `EXPR`이어야 함
- `outputIds`에서 참조하는 아웃풋 ID는 `outputs` 목록에 존재해야 함
- `storage.type`이 `SQLITE`인 경우 `storage.path`가 비어 있지 않아야 함
- `queue.type`이 `FILE`인 경우 `queue.path`가 비어 있지 않아야 함

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
헤더: `Location: /inputs/{inputId}/messages/{messageId}`

### 메시지 조회

ID로 메시지를 조회한다. `inputId`는 인증(Bearer 토큰 조회)에만 사용되며, 메시지 조회는 `messageId`만으로 이루어진다.

```
GET /inputs/{inputId}/messages/{messageId}
Authorization: Bearer <secret>
```

응답 `200 OK`:
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

**`status` 값:**

| 값 | 설명 |
|----|------|
| `PENDING` | 큐에 등록됨, 아직 처리되지 않음 |
| `DELIVERED` | 매칭된 모든 아웃풋에 성공적으로 전달됨 |
| `FAILED` | 모든 재시도 소진, 전달 실패 |

참고: `lastAttemptAt`는 전달 시도가 없으면 응답에서 생략된다.

**에러 응답 (RFC 7807):**

| 상태 코드 | 조건 |
|-----------|------|
| `401 Unauthorized` | Bearer 토큰 없음 또는 유효하지 않음 |
| `404 Not Found` | 메시지 ID가 존재하지 않음 |

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

### API 문서

```
GET /docs          → Redoc HTML UI
GET /docs/openapi  → OpenAPI 스펙 (JSON)
GET /docs/asyncapi → AsyncAPI 스펙 (JSON)
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

| 경로 | 역할 |
|------|------|
| `internal/domain/` | 엔티티(`Message`, `Output`), 열거형(`MessageStatus`, `OutputType`), 센티넬 에러 |
| `internal/application/port/input/` | `ReceiveMessageUseCase`, `GetMessageUseCase` 인터페이스 |
| `internal/application/port/output/` | `MessageRepository`, `MessageQueue`, `OutputSender`, `OutputRegistry`, `RuleConfigReader` 인터페이스 |
| `internal/application/service/` | `MessageService`(Receive, GetByID), `RelayWorker`(Start) |
| `internal/config/` | Viper 기반 YAML 로더, `InMemoryRuleConfigReader`, hot-reload(`Watch`) |
| `internal/adapter/input/http/` | chi 라우터, RFC 7807 에러, `X-API-Version` 헤더 미들웨어 |
| `internal/adapter/input/websocket/` | gorilla/websocket 인바운드 핸들러 |
| `internal/adapter/output/sqlite/` | sqlc 기반 SQLite 저장소 |
| `internal/adapter/output/filequeue/` | 파일 기반 at-least-once 큐 |
| `internal/adapter/output/webhook/` | HTTP Webhook 송신 |
| `cmd/server/` | cobra `start` 커맨드, 전체 DI 조립 |
| `test/e2e/` | 전체 흐름 E2E 테스트 |

## 릴리스

버전 태그를 푸시하면 GitHub Actions가 자동으로 모든 플랫폼 바이너리를 빌드하고 GitHub Release를 생성한다.

```bash
git tag 1.0.0
git push origin 1.0.0
```

지원 플랫폼: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, `windows/arm64`

각 릴리스에는 `checksums.txt`에 SHA256 체크섬이 포함된다.

## 개발

```bash
# 전체 테스트 (race detector 포함)
go test -race ./... -timeout 60s

# 정적 분석
go vet ./...

# sqlc 코드 재생성 (SQL 변경 후)
cd internal/adapter/output/sqlite && sqlc generate

# 현재 플랫폼 빌드
make build

# 전체 플랫폼 크로스 컴파일
make build-all
```

# 설정 레퍼런스

relaybox는 YAML 파일로 설정합니다. 기본 파일명은 `config.yaml`이며, `--config` 플래그로 경로를 지정할 수 있습니다.

## CLI 플래그

| 플래그 | 단축 | 기본값 | 설명 |
|--------|------|--------|------|
| `--config` | `-c` | `"config.yaml"` | 설정 파일 경로 |

---

## server

HTTP 서버 설정.

| YAML 키 | 타입 | 기본값 | 필수 | 설명 |
|---------|------|--------|------|------|
| `server.port` | int | `8080` | | 바인드 포트 |
| `server.readTimeout` | duration | `"30s"` | | 읽기 타임아웃 |
| `server.writeTimeout` | duration | `"30s"` | | 쓰기 타임아웃 |

```yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
```

### server.tls

TLS 설정 (현재 구현 예정).

| YAML 키 | 타입 | 기본값 | 필수 | 설명 |
|---------|------|--------|------|------|
| `server.tls.enabled` | bool | `false` | | TLS 활성화 여부 |
| `server.tls.certFile` | string | `""` | | TLS 인증서 파일 경로 |
| `server.tls.keyFile` | string | `""` | | TLS 키 파일 경로 |

```yaml
server:
  tls:
    enabled: false
    certFile: ""
    keyFile: ""
```

---

## log

로깅 설정.

| YAML 키 | 타입 | 기본값 | 허용값 | 설명 |
|---------|------|--------|--------|------|
| `log.level` | string | `"INFO"` | `DEBUG` `INFO` `WARN` `ERROR` | 로그 레벨 |
| `log.format` | string | `"JSON"` | `JSON` `TEXT` | 로그 출력 형식 |

```yaml
log:
  level: INFO
  format: JSON
```

---

## inputs[]

인바운드 엔드포인트 목록. HTTP와 TCP 두 가지 방식을 지원합니다.

각 input은 `rules[]` 배열을 내장합니다. 하나의 input에 여러 규칙을 정의할 수 있으며, 각 규칙은 독립적으로 평가됩니다.

| YAML 키 | 타입 | 기본값 | 필수 | 설명 |
|---------|------|--------|------|------|
| `inputs[].id` | string | — | **필수** | 입력 식별자. 전체에서 유일해야 함. CEL 표현식에서 `data.input`으로 참조됨 |
| `inputs[].engine` | string | — | **필수** | filter/mapping/routing 평가에 사용할 표현식 엔진. 허용값: `CEL` `EXPR` |
| `inputs[].parser` | string | `""` | | 페이로드 파서. 허용값: `JSON` `FORM` `XML` `LOGFMT` `REGEX` |
| `inputs[].secret` | string | `""` | | Bearer 토큰 인증 (`Authorization: Bearer <secret>`). HTTP 입력에만 적용 |
| `inputs[].pattern` | string | `""` | | `parser: REGEX` 사용 시 정규식 패턴 |
| `inputs[].address` | string | `""` | | TCP 바인드 주소 (예: `":9001"`). 지정 시 TCP 리스너를 시작함 |
| `inputs[].delimiter` | string | `""` | | TCP 메시지 구분자. 1글자만 허용. `address` 지정 시 기본값 `"\n"` |

### inputs[].rules[]

| YAML 키 | 타입 | 기본값 | 필수 | 설명 |
|---------|------|--------|------|------|
| `inputs[].rules[].outputIds` | []string | `[]` | | 단순 모드: 필터/라우팅 없이 지정 출력으로 전달 |
| `inputs[].rules[].filter` | string | `""` | | 전달 여부를 결정하는 boolean 표현식. 빈 값이면 모든 메시지 통과 |
| `inputs[].rules[].mapping` | map[string]string | `{}` | | 메시지 페이로드에 필드를 추가/덮어쓰기. 키→표현식 |
| `inputs[].rules[].routing` | []RouteConditionConfig | `[]` | | 조건별 출력 라우팅 |

### inputs[].rules[].routing[]

| YAML 키 | 타입 | 기본값 | 필수 | 설명 |
|---------|------|--------|------|------|
| `inputs[].rules[].routing[].condition` | string | — | **필수** | boolean 표현식. true일 때 해당 출력으로 전달 |
| `inputs[].rules[].routing[].outputIds` | []string | — | **필수** | condition이 true일 때 전달할 `outputs[].id` 목록 |

```yaml
inputs:
  # HTTP 입력 — 단순 모드 (필터 없이 전달)
  - id: dozzle
    engine: CEL
    parser: JSON
    secret: "change-me"
    rules:
      - outputIds: [ops-webhook]

  # HTTP 입력 — 조건부 라우팅
  - id: beszel
    engine: CEL
    parser: JSON
    secret: "change-me"
    rules:
      - filter: 'data.severity == "HIGH"'
        mapping:
          level: '"CRITICAL"'
        routing:
          - condition: 'data.level == "CRITICAL"'
            outputIds: [ops-webhook]
      - outputIds: [naver-works-bot]   # 두 번째 규칙: 항상 알림봇으로 전달

  # TCP 입력
  - id: tcp-input
    engine: CEL
    address: ":9001"
    delimiter: "\n"
    parser: JSON
    rules:
      - outputIds: [ops-webhook]
```

---

## outputs[]

아웃바운드 대상 목록.

| YAML 키 | 타입 | 기본값 | 필수 | 설명 |
|---------|------|--------|------|------|
| `outputs[].id` | string | — | **필수** | 출력 식별자. `rules[].outputIds`에서 참조. 전체에서 유일해야 함 |
| `outputs[].type` | string | — | **필수** | 출력 타입. 허용값: `WEBHOOK` |
| `outputs[].engine` | string | — | **필수** | template 평가에 사용할 표현식 엔진. 허용값: `CEL` `EXPR` |
| `outputs[].url` | string | `""` | | 대상 URL |
| `outputs[].template` | map[string]string | `{}` | | 페이로드 템플릿. 키는 dot-notation으로 중첩 JSON 생성, 값은 표현식 |
| `outputs[].secret` | string | `""` | | 아웃바운드 요청에 추가할 Bearer 토큰 |
| `outputs[].retryCount` | int | `0` | | 재시도 횟수 (0이면 `worker.defaultRetryCount` 사용) |
| `outputs[].retryDelayMs` | int | `0` | | 재시도 기본 지연 시간(ms) (0이면 `worker.defaultRetryDelay` 사용) |
| `outputs[].timeoutSec` | int | `0` | | HTTP 요청 타임아웃(초). 0이면 제한 없음 |
| `outputs[].skipTLSVerify` | bool | `false` | | TLS 인증서 검증 무시 여부 |

### template dot-notation

`template` 키에서 `.`으로 구분된 키를 사용하면 중첩 JSON을 생성합니다.

```yaml
outputs:
  - id: ops-webhook
    type: WEBHOOK
    engine: CEL
    url: "https://hooks.example.com/xyz"
    template:
      text: 'data.input + ": " + data.payload'
    retryCount: 3
    retryDelayMs: 1000
    timeoutSec: 10
    skipTLSVerify: false

  # dot-notation 예시 — {"content": {"type": "text", "text": "..."}} 생성
  - id: naver-works-bot
    type: WEBHOOK
    engine: CEL
    url: "https://naver-works-bot.example.com/api/v1/bots/1/text"
    secret: "change-me-jwt-token"
    template:
      content.type: '"text"'
      content.text: 'data.input + " alert: " + data.payload'
```

---

## storage

메시지 저장소 설정.

| YAML 키 | 타입 | 기본값 | 허용값 | 설명 |
|---------|------|--------|--------|------|
| `storage.type` | string | `""` | `SQLITE` | 저장소 종류 |
| `storage.path` | string | `""` | | DB 파일 경로. `":memory:"` 지정 시 인메모리 |

```yaml
storage:
  type: SQLITE
  path: "./data/relaybox.db"
```

---

## queue

전달 큐 설정.

| YAML 키 | 타입 | 기본값 | 허용값 | 설명 |
|---------|------|--------|--------|------|
| `queue.type` | string | `""` | `FILE` | 큐 종류 |
| `queue.path` | string | `""` | | 파일 큐 디렉토리 경로 |
| `queue.workerCount` | int | `2` | | 릴레이 워커 고루틴 수 |

```yaml
queue:
  type: FILE
  path: "./data/queue"
  workerCount: 2
```

---

## worker

릴레이 워커 동작 설정.

| YAML 키 | 타입 | 기본값 | 설명 |
|---------|------|--------|------|
| `worker.defaultRetryCount` | int | `3` | 출력에 `retryCount`가 없을 때 사용할 기본 재시도 횟수 |
| `worker.defaultRetryDelay` | duration | `"1s"` | 출력에 `retryDelayMs`가 없을 때 사용할 기본 재시도 지연 시간 |
| `worker.pollBackoff` | duration | `"500ms"` | 큐가 비었을 때 다음 폴링까지 대기 시간 |

```yaml
worker:
  defaultRetryCount: 3
  defaultRetryDelay: "1s"
  pollBackoff: "500ms"
```

---

## Hot-Reload

설정 파일 변경 시 일부 섹션은 서버 재시작 없이 반영됩니다.

| 섹션 | 재시작 없이 반영 | 비고 |
|------|:---:|------|
| `inputs[].rules[]` | O | 라우팅 규칙 변경 즉시 적용 |
| `outputs` | O | 출력 대상 변경 즉시 적용 |
| `server` | X | 포트/타임아웃 변경은 재시작 필요 |
| `log` | X | |
| `inputs` (engine/parser 등) | X | |
| `storage` | X | |
| `queue` | X | |
| `worker` | X | |

---

## 전체 예시

전체 설정 예시는 [`docs/config.example.yaml`](config.example.yaml)을 참고하세요.

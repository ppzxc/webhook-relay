# API Documentation Design Spec

**Date:** 2026-03-20
**Status:** Approved

---

## 1. 개요

webhook-relay REST API 및 WebSocket 인바운드를 문서화하는 정적 스펙 파일 + 서버 내장 Redoc UI.

**핵심 목표:**
- OpenAPI 3.1.0 (hand-written) + AsyncAPI 3.0 으로 전체 API 문서화
- 단일 바이너리에 embed된 Redoc UI — 외부 CDN 의존 없음
- `/docs`, `/docs/openapi`, `/docs/asyncapi` 엔드포인트로 서빙
- GitHub Actions CI에서 spectral lint + asyncapi validate 자동 검증

---

## 2. 파일 구조

```
internal/apidocs/
├── embed.go              # //go:embed 선언 + DocsHandler (package apidocs)
├── openapi.yaml          # OpenAPI 3.1.0 (hand-written)
├── asyncapi.yaml         # AsyncAPI 3.0 (WebSocket 인바운드)
└── redoc.standalone.js   # Redoc v2 standalone (~1MB, embed용)

internal/adapter/input/http/
└── router.go             # GET /docs, /docs/openapi, /docs/asyncapi 추가

.github/workflows/
└── api-doc.yml           # spectral lint + asyncapi validate

.spectral.yaml            # OAS 3.1 ruleset 설정
```

> **embed 경로 제약:** Go `//go:embed`는 `..` 경로를 허용하지 않는다.
> 따라서 스펙 파일과 `embed.go`를 동일 패키지(`internal/apidocs/`) 아래에 배치한다.
> HTTP 어댑터는 `apidocs` 패키지를 import하여 핸들러를 받는다.

---

## 3. HTTP 라우트

| Method | URL | Content-Type 응답 | 인증 | 설명 |
|--------|-----|-------------------|------|------|
| `GET` | `/docs` | `text/html; charset=utf-8` | 없음 | Redoc HTML |
| `GET` | `/docs/openapi` | `application/yaml` | 없음 | raw OpenAPI YAML |
| `GET` | `/docs/asyncapi` | `application/yaml` | 없음 | raw AsyncAPI YAML |

- URL에 파일 확장자 없음 (REST API 가이드라인: No file extensions)
- 공개 엔드포인트 — Bearer token 인증 불필요
- `NewRouter`에 `/docs` 하위 route group 추가로 통합

---

## 4. embed 및 핸들러 구조

```go
// internal/apidocs/embed.go
package apidocs

import _ "embed"

//go:embed openapi.yaml
var OpenAPISpec []byte

//go:embed asyncapi.yaml
var AsyncAPISpec []byte

//go:embed redoc.standalone.js
var RedocJS []byte

// DocsHandler returns chi-compatible handlers
func OpenAPIHandler(w http.ResponseWriter, r *http.Request) { ... }
func AsyncAPIHandler(w http.ResponseWriter, r *http.Request) { ... }
func RedocHTMLHandler(w http.ResponseWriter, r *http.Request) { ... }
```

**`GET /docs` HTML 구조 (Redoc 초기화):**
```html
<!DOCTYPE html>
<html>
  <head><title>webhook-relay API</title></head>
  <body>
    <redoc spec-url="/docs/openapi"></redoc>
    <script>/* inlined redoc.standalone.js */</script>
  </body>
</html>
```

- `spec-url="/docs/openapi"` — 브라우저가 서버에서 YAML을 fetch
- Redoc JS는 인라인 `<script>` 태그로 삽입 (CDN 참조 없음)

---

## 5. OpenAPI 3.1.0 스펙

**버전:** 3.1.0

**인증:**
```yaml
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
```

**공통 스키마:**
- `Alert` — id, sourceId, status (AlertStatus enum), createdAt, retryCount
- `AlertCreatedResponse` — 201 응답 body (id, sourceId, status, createdAt)
- `Problem` — RFC 7807/9457 에러 응답 (type, title, status, detail, traceId)

**엔드포인트 목록:**

| Method | Path | 구현 상태 | 설명 |
|--------|------|-----------|------|
| `GET` | `/healthz` | ✅ 구현 | 헬스체크 |
| `POST` | `/sources/{sourceId}/alerts` | ✅ 구현 | 알람 수신 (201 + Location) |
| `GET` | `/sources/{sourceId}/alerts/{alertId}` | ✅ 구현 (placeholder) | 알람 단건 조회 |
| `GET` | `/sources/{sourceId}/alerts` | 🔲 planned | 알람 목록 조회 |
| `PATCH` | `/sources/{sourceId}/alerts/{alertId}` | 🔲 planned | 상태 변경 (FAILED→PENDING) |
| `GET` | `/sources` | 🔲 planned | 소스 목록 |
| `GET` | `/sources/{sourceId}` | 🔲 planned | 소스 단건 |
| `GET` | `/channels` | 🔲 planned | 채널 목록 |
| `GET` | `/channels/{channelId}` | 🔲 planned | 채널 단건 |

미구현 엔드포인트: `x-status: "planned"` 확장 필드로 표시.
WebSocket (`/sources/{sourceId}/alerts/ws`)은 OpenAPI에서 제외 — AsyncAPI에서 문서화.

---

## 6. AsyncAPI 3.0 스펙

**버전:** 3.0.0

**채널:** `ws /sources/{sourceId}/alerts/ws`

**방향 (AsyncAPI 3.0 action 기준):**
- 서버 입장에서 `receive` (클라이언트가 Alert 페이로드를 서버로 push)
- 모니터링 앱(Beszel, Dozzle 등)이 WebSocket으로 연결 후 알람을 전송
- 서버는 수신 즉시 `ReceiveAlertUseCase.Receive()` 호출

**메시지 스키마:** Alert JSON 페이로드 (소스별 원시 JSON)

**인증:** Bearer token (`Authorization` 헤더, WebSocket 연결 시)

---

## 7. GitHub Actions 파이프라인

```yaml
# .github/workflows/api-doc.yml
name: Validate API Specs

on:
  push:
    branches: [main]
    paths: ['internal/apidocs/**']
  pull_request:
    paths: ['internal/apidocs/**']

jobs:
  validate-openapi:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Spectral lint
        run: npx --yes @stoplight/spectral-cli@6 lint internal/apidocs/openapi.yaml --ruleset .spectral.yaml

  validate-asyncapi:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: AsyncAPI validate
        run: npx --yes @asyncapi/cli@3 validate internal/apidocs/asyncapi.yaml
```

**`.spectral.yaml`:**
```yaml
extends: ["spectral:oas"]
rules:
  oas3-valid-schema: error
  operation-description: warn
  operation-tags: warn
```

- Spectral CLI: `@stoplight/spectral-cli@6` (고정)
- AsyncAPI CLI: `@asyncapi/cli@3` (고정)
- PR 시 스펙 파일 변경이 있으면 lint 통과 필수 (required check)

---

## 8. NewRouter 통합

```go
// internal/adapter/input/http/router.go
import "webhook-relay/internal/apidocs"

func NewRouter(uc input.ReceiveAlertUseCase, resolver SourceResolver, ws WSHandler) *chi.Mux {
    // ... 기존 미들웨어 ...

    r.Get("/docs", apidocs.RedocHTMLHandler)
    r.Get("/docs/openapi", apidocs.OpenAPIHandler)
    r.Get("/docs/asyncapi", apidocs.AsyncAPIHandler)

    // ... 기존 라우트 ...
}
```

---

## 9. 기술 스택 추가

| 역할 | 도구 | 버전 |
|------|------|------|
| REST API 문서 | OpenAPI | 3.1.0 |
| WebSocket 문서 | AsyncAPI | 3.0.0 |
| UI 렌더러 | Redoc standalone | v2 |
| 스펙 검증 | Spectral CLI | @6 |
| AsyncAPI 검증 | AsyncAPI CLI | @3 |
| CI | GitHub Actions | — |

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
api/
├── openapi.yaml          # OpenAPI 3.1.0 (hand-written)
├── asyncapi.yaml         # AsyncAPI 3.0 (WebSocket 인바운드)
└── redoc.standalone.js   # Redoc v2 standalone (~1MB, embed용)

internal/adapter/input/http/
├── docs.go               # //go:embed + DocsHandler
└── router.go             # GET /docs, /docs/openapi, /docs/asyncapi 추가

.github/workflows/
└── api-doc.yml           # spectral lint + asyncapi validate

.spectral.yaml            # OAS 3.1 ruleset 설정
```

---

## 3. HTTP 라우트

| Method | URL | 설명 | 인증 |
|--------|-----|------|------|
| `GET` | `/docs` | Redoc HTML (OpenAPI 렌더링) | 없음 |
| `GET` | `/docs/openapi` | raw OpenAPI YAML (`application/yaml`) | 없음 |
| `GET` | `/docs/asyncapi` | raw AsyncAPI YAML (`application/yaml`) | 없음 |

- URL에 파일 확장자 없음 (REST API 가이드라인 준수)
- 공개 엔드포인트 — Bearer token 인증 불필요

---

## 4. OpenAPI 3.1.0 스펙

**버전:** 3.1.0

**인증:**
```yaml
securitySchemes:
  bearerAuth:
    type: http
    scheme: bearer
```

**공통 스키마:**
- `Alert` — id, sourceId, status (AlertStatus enum), createdAt
- `AlertCreatedResponse` — 201 응답 body
- `Problem` — RFC 7807/9457 에러 응답

**엔드포인트 목록:**

| Method | Path | 구현 상태 | 설명 |
|--------|------|-----------|------|
| `GET` | `/healthz` | ✅ 구현 | 헬스체크 |
| `POST` | `/sources/{sourceId}/alerts` | ✅ 구현 | 알람 수신 |
| `GET` | `/sources/{sourceId}/alerts/{alertId}` | 🔲 planned | 알람 단건 조회 |
| `GET` | `/sources/{sourceId}/alerts` | 🔲 planned | 알람 목록 조회 |
| `PATCH` | `/sources/{sourceId}/alerts/{alertId}` | 🔲 planned | 상태 변경 (FAILED→PENDING) |
| `GET` | `/sources` | 🔲 planned | 소스 목록 |
| `GET` | `/sources/{sourceId}` | 🔲 planned | 소스 단건 |
| `GET` | `/channels` | 🔲 planned | 채널 목록 |
| `GET` | `/channels/{channelId}` | 🔲 planned | 채널 단건 |

미구현 엔드포인트: `x-status: "planned"` 확장 필드로 표시.

WebSocket (`/sources/{sourceId}/alerts/ws`)은 OpenAPI에서 제외 — AsyncAPI에서 문서화.

---

## 5. AsyncAPI 3.0 스펙

**버전:** 3.0.0

**채널:** `ws /sources/{sourceId}/alerts/ws`

**방향:** 인바운드 전용
- 모니터링 앱(Beszel, Dozzle 등)이 WebSocket으로 서버에 연결
- 클라이언트가 Alert 페이로드를 서버로 push
- 서버는 동일한 `ReceiveAlertUseCase.Receive()` 호출

**메시지 스키마:** Alert JSON 페이로드 (소스별 원시 페이로드)

**인증:** Bearer token (WebSocket 연결 시 `Authorization` 헤더)

---

## 6. embed 방식

```go
// internal/adapter/input/http/docs.go
//go:embed ../../../api/openapi.yaml
var openapiSpec []byte

//go:embed ../../../api/asyncapi.yaml
var asyncapiSpec []byte

//go:embed ../../../api/redoc.standalone.js
var redocJS []byte
```

- Redoc HTML은 코드에서 인라인 생성 (CDN 참조 없음)
- `GET /docs` — `text/html` 반환, 인라인 `<script>` 로 redocJS 참조
- `GET /docs/openapi` — `application/yaml` 반환
- `GET /docs/asyncapi` — `application/yaml` 반환

---

## 7. GitHub Actions 파이프라인

```yaml
# .github/workflows/api-doc.yml
# 트리거: api/openapi.yaml 또는 api/asyncapi.yaml 변경 시 (PR + push to main)

jobs:
  validate-openapi:
    - npx @stoplight/spectral-cli lint api/openapi.yaml --ruleset .spectral.yaml

  validate-asyncapi:
    - npx @asyncapi/cli validate api/asyncapi.yaml
```

`.spectral.yaml`:
```yaml
extends: ["spectral:oas"]
rules:
  oas3-valid-schema: error
  operation-description: warn
  operation-tags: warn
```

PR 시 스펙 파일 변경이 있으면 lint 통과 필수 (required check).

---

## 8. 기술 스택 추가

| 역할 | 도구 |
|------|------|
| REST API 문서 | OpenAPI 3.1.0 (YAML) |
| WebSocket 문서 | AsyncAPI 3.0 (YAML) |
| UI 렌더러 | Redoc v2 (embed) |
| 스펙 검증 | Spectral CLI (OAS), AsyncAPI CLI |
| CI | GitHub Actions |

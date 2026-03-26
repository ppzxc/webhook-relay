---
status: accepted
date: 2026-01-01
---

# ADR-0013: URL 버저닝 대신 X-API-Version 응답 헤더 사용

## Context and Problem Statement

API 버전을 클라이언트에 알려야 한다. URL 버저닝(/v1/..., /v2/...)은 라우팅 복잡도를 높이고 기존 클라이언트 URL을 변경해야 하는 부담을 준다. 버전 정보를 전달하면서도 URL 구조를 단순하게 유지하는 방법이 필요하다.

## Considered Options

* **X-API-Version 응답 헤더** — 모든 응답에 X-API-Version 헤더를 포함하여 현재 API 버전을 클라이언트에 전달
* **URL 경로 버저닝 (/v1/...)** — URL 경로에 버전을 포함하여 명시적으로 버전별 라우팅을 구분
* **Accept 헤더 버저닝** — 요청의 Accept 헤더(application/vnd.relaybox.v1+json 등)로 버전을 협상

## Decision Outcome

Chosen option: "X-API-Version 응답 헤더", because URL 구조를 단순하게 유지하면서 클라이언트가 현재 서버 버전을 쉽게 확인할 수 있으며, 버전 업그레이드 시 클라이언트 URL 수정이 불필요하기 때문이다.

## Consequences

* Good, because URL 구조가 단순하게 유지되어 라우팅 복잡도가 낮음
* Good, because API 버전이 올라가도 기존 클라이언트의 URL을 수정할 필요가 없음
* Bad, because 응답 헤더에만 버전이 포함되므로 클라이언트가 특정 버전을 요청에 명시하여 버전을 협상할 수 없음

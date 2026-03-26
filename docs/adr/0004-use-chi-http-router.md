---
status: accepted
date: 2026-01-01
---

# ADR-0004: chi를 HTTP 라우터로 선택

## Context and Problem Statement

HTTP 핸들러 등록, 미들웨어 체인, URL 파라미터 추출이 필요하다. Go stdlib `net/http`는 패턴 매칭이 제한적이고 URL 파라미터 추출을 위한 별도 처리가 필요하다. 외부 라우터를 도입할 경우 stdlib 호환성과 유지보수 활성도를 함께 고려해야 한다.

## Considered Options

* **go-chi/chi** — stdlib `net/http` 완전 호환, 미들웨어 중심 설계의 경량 라우터
* **gorilla/mux** — 성숙한 기능을 갖춘 라우터이나 2022년 아카이브 후 유지보수 중단
* **stdlib net/http (Go 1.22+)** — Go 1.22에서 개선된 패턴 매칭을 활용하는 의존성 제로 방식

## Decision Outcome

Chosen option: "go-chi/chi", because stdlib `net/http` 완전 호환으로 어댑터 교체가 용이하고, 미들웨어 체인이 간결하며 활발한 유지보수가 보장된다.

## Consequences

* Good, because stdlib `net/http` 완전 호환, 미들웨어 체인 간결, 활발한 유지보수로 장기적 안정성이 확보된다.
* Bad, because 외부 의존성이 추가된다 (경량이지만 stdlib 대비 추가 모듈).

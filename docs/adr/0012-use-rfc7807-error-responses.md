---
status: accepted
date: 2026-01-01
---

# ADR-0012: RFC 7807 Problem Details 에러 응답 형식

## Context and Problem Statement

HTTP API에서 에러를 클라이언트에 전달할 때 일관된 형식이 필요하다. 커스텀 에러 포맷은 클라이언트 파싱 구현마다 달라지는 문제가 있다. 다양한 클라이언트(webhook 발신자, 관리 도구 등)가 동일한 방식으로 에러를 처리할 수 있어야 한다.

## Considered Options

* **RFC 7807 Problem Details (application/problem+json)** — IETF 표준 에러 응답 형식으로 type/title/status/detail 필드를 포함
* **커스텀 JSON 에러 포맷** — 프로젝트 전용 에러 구조체를 정의하여 JSON으로 직렬화
* **단순 HTTP 상태 코드만** — 에러 본문 없이 HTTP 상태 코드만으로 에러를 표현

## Decision Outcome

Chosen option: "RFC 7807 Problem Details (application/problem+json)", because IETF 표준을 따름으로써 별도 문서 없이도 클라이언트가 에러 구조를 예측할 수 있고, Content-Type 헤더로 에러 응답을 즉시 식별할 수 있기 때문이다.

## Consequences

* Good, because 표준 형식으로 클라이언트 호환성이 높으며 별도 에러 파싱 로직 구현 부담이 줄어듦
* Good, because Content-Type: application/problem+json으로 일반 응답과 에러 응답을 명확히 구분할 수 있음
* Good, because type/title/status/detail 필드가 표준화되어 에러 종류와 상세 내용을 일관되게 전달할 수 있음
* Bad, because RFC 7807 필드 이름(type, title 등)이 도메인 언어와 다소 이질적으로 느껴질 수 있음

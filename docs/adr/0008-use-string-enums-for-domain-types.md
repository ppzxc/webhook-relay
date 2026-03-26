---
status: accepted
date: 2026-01-01
---

# ADR-0008: string 기반 도메인 열거형

## Context and Problem Statement

`MessageStatus`(PENDING/DELIVERED/FAILED), `OutputType`(WEBHOOK/SLACK/DISCORD) 같은 도메인 열거형을 정의해야 한다. JSON 직렬화 시 사람이 읽을 수 있어야 하고 DB에 저장할 때도 의미가 있어야 한다.

## Considered Options

* **type X string + 대문자 상수** — `type MessageStatus string`으로 선언하고 `"PENDING"` 같은 문자열 상수 사용
* **type X int + iota** — 정수 기반 열거형으로 선언
* **protobuf-style enum** — protobuf 정의에서 코드 생성

## Decision Outcome

Chosen option: "type X string + 대문자 상수 (\"PENDING\", \"DELIVERED\" 등)", because JSON/DB 저장 시 별도 `MarshalJSON` 구현이 불필요하고 로그와 DB에서 값을 즉시 읽을 수 있다.

## Consequences

* Good, because JSON/DB 저장 시 별도 MarshalJSON 불필요, 로그에서 즉시 읽기 가능, 타입 안전성 유지
* Bad, because string 비교이므로 int 비교보다 미미하게 느림 (무시 가능한 수준)

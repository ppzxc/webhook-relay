---
status: accepted
date: 2026-02-20
---

# ADR-0018: InputType 열거형 제거, input ID를 라우팅 키로 사용

## Context and Problem Statement

초기 설계에서 InputType 열거형(HTTP, WEBSOCKET, TCP)을 라우팅 키로 사용했다. 그러나 동일 프로토콜의 여러 입력을 구별할 수 없었고, 입력 구분이 프로토콜보다 설정 ID 기반이 더 자연스러웠다. 예를 들어 두 개의 HTTP 입력을 운영할 때 InputType만으로는 각각을 독립적으로 라우팅할 수 없었다.

## Considered Options

* **input ID (설정 파일의 inputs[].id 값)** — 각 입력을 고유 문자열 ID로 식별하고 이를 라우팅 키로 사용
* **InputType 열거형 유지** — HTTP/WEBSOCKET/TCP 등 프로토콜 타입을 라우팅 키로 계속 사용
* **복합 키 (type + id)** — 프로토콜 타입과 ID를 조합하여 라우팅 키 구성

## Decision Outcome

Chosen option: "input ID만 사용", because 동일 프로토콜의 여러 입력을 구별할 수 있고, 설정 파일과 코드가 일관되며, CEL 표현식에서 `data.input`으로 접근 시 의미가 명확하기 때문이다.

## Consequences

* Good, because 동일 프로토콜 여러 입력 구별 가능, 설정 파일과 코드가 일관됨, CEL 표현식에서 `data.input`으로 접근 시 의미가 명확
* Bad, because 열거형 제거로 기존 코드 변경 필요 (커밋 ad946e8에서 리팩터링)

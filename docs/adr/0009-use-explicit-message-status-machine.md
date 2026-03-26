---
status: accepted
date: 2026-01-01
---

# ADR-0009: 명시적 메시지 상태 기계

## Context and Problem Statement

메시지는 PENDING → DELIVERED 또는 PENDING → FAILED → PENDING(requeue) 상태 전이를 따른다. 잘못된 전이(DELIVERED → PENDING 등)를 방지해야 한다. `domain.MessageStatus`에 `CanTransitionTo()` 메서드를 두고 허용 전이(PENDING→DELIVERED, PENDING→FAILED, FAILED→PENDING)를 명시적으로 정의한다.

## Considered Options

* **명시적 CanTransitionTo() 메서드** — 도메인 타입에 상태 전이 유효성 검사 메서드를 추가
* **런타임 검사 없이 컨벤션으로** — 허용 전이를 문서화하고 개발자 규율에 의존
* **DB 레벨 constraint** — CHECK constraint로 유효하지 않은 상태값 저장 방지

## Decision Outcome

Chosen option: "명시적 CanTransitionTo() 메서드", because 잘못된 상태 전이 시 `ErrInvalidTransition`을 반환하여 빠른 실패가 가능하고 허용 전이가 도메인 코드 자체에 문서화된다.

## Consequences

* Good, because 잘못된 상태 전이 시 ErrInvalidTransition 반환으로 빠른 실패, 허용 전이가 도메인 코드에 문서화됨
* Bad, because 새 상태 추가 시 CanTransitionTo() 업데이트 필요

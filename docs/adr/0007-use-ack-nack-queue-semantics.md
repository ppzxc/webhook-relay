---
status: accepted
date: 2026-01-01
---

# ADR-0007: Ack/Nack 큐 소비 패턴

## Context and Problem Statement

웹훅 전달 실패 시 메시지를 큐에 되돌려 재시도해야 한다. 소비자가 처리 중 크래시하면 메시지가 유실되면 안 된다. Dequeue 시 `.json` → `.json.processing`으로 rename, Ack는 `.processing` 삭제, Nack는 `.json`으로 rename 복구, 서버 시작 시 `.processing` 파일을 `.json`으로 복원하는 방식으로 구현된다.

## Considered Options

* **Ack/Nack 콜백 패턴** — Dequeue 시 AckFunc/NackFunc를 함께 반환하여 소비자가 명시적으로 처리 결과를 통보
* **단순 Dequeue-Delete 패턴** — Dequeue 즉시 삭제하고 실패 시 재삽입
* **트랜잭션 기반** — DB 트랜잭션으로 원자적 처리

## Decision Outcome

Chosen option: "Ack/Nack 콜백 패턴", because 처리 완료 전까지 메시지를 보존하여 크래시 후 자동 복구가 가능하고, 인터페이스가 RabbitMQ 등 메시지 브로커와 개념적으로 호환된다.

## Consequences

* Good, because at-least-once 전달 보장, 소비자 크래시 후 자동 복구, 인터페이스가 메시지 브로커(RabbitMQ 등)와 개념적으로 호환
* Bad, because at-least-once이므로 소비자가 멱등성을 보장해야 함

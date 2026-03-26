---
status: accepted
date: 2026-01-01
---

# ADR-0006: 파일 기반 메시지 큐

## Context and Problem Statement

HTTP로 수신한 메시지를 즉시 웹훅으로 전달하지 않고 큐에 넣어 비동기로 처리해야 한다. 외부 브로커(Kafka, RabbitMQ) 없이 단일 바이너리로 운영하고 싶다.

## Considered Options

* **파일시스템 기반 큐 (JSON 파일)** — 메시지를 JSON 파일로 디스크에 저장하고 워커가 순차적으로 소비
* **Redis 기반 큐** — Redis List를 큐로 사용하여 고속 처리
* **In-memory 큐 (채널)** — Go 채널을 이용한 메모리 내 큐

## Decision Outcome

Chosen option: "파일시스템 기반 큐 (JSON 파일)", because 외부 의존성 없이 단일 바이너리로 배포할 수 있고 프로세스 재시작 후에도 미처리 메시지를 복구할 수 있다.

## Consequences

* Good, because 외부 의존성 없음, 프로세스 재시작 후에도 미처리 메시지 복구 가능(at-least-once), 큐 내용을 파일로 직접 확인 가능
* Bad, because 파일 I/O로 인한 처리량 제한, NFS 등 분산 파일시스템에서 경쟁 조건 가능성

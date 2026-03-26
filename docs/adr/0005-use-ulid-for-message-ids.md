---
status: accepted
date: 2026-01-01
---

# ADR-0005: ULID를 메시지 ID로 사용

## Context and Problem Statement

메시지를 고유하게 식별하고 URL에 노출되는 ID가 필요하다. ID 생성 시 DB 조회 없이 애플리케이션 레이어에서 생성 가능해야 하며, 대량 메시지 처리 시 충돌 없이 분산 생성이 가능해야 한다.

## Considered Options

* **ULID (oklog/ulid)** — 타임스탬프(48bit) + 랜덤(80bit) 조합의 128bit URL-safe ID
* **UUID v4 (랜덤)** — 완전 랜덤 128bit ID, 업계 표준
* **Auto-increment integer** — DB에서 순차 발급하는 정수 ID

## Decision Outcome

Chosen option: "ULID", because 시간순 정렬이 가능하여 메시지 조회 시 인덱스 효율이 높고, URL-safe 문자셋으로 별도 인코딩 없이 URL에 직접 사용 가능하며, DB 없이 애플리케이션 레이어에서 생성할 수 있어 분산 환경에도 적합하다.

## Consequences

* Good, because 시간순 정렬 가능(lexicographic), URL-safe, DB 없이 생성 가능, 충돌 확률이 극히 낮다.
* Bad, because UUID보다 덜 알려진 형식으로 외부 시스템과의 호환성 논의가 필요할 수 있다.

---
status: accepted
date: 2026-01-01
---

# ADR-0002: 헥사고날 아키텍처 채택

## Context and Problem Statement

웹훅 릴레이 허브는 다양한 입력(HTTP, WebSocket, TCP)과 출력(Webhook, Slack 등), 스토리지(SQLite, MariaDB)를 교체 가능하게 지원해야 한다. 외부 의존성이 도메인 로직에 침투하면 테스트와 어댑터 교체가 어렵다. 장기적으로 외부 시스템 변경이 비즈니스 로직에 영향을 주지 않도록 격리가 필요하다.

## Considered Options

* **Hexagonal Architecture (Ports & Adapters)** — 도메인을 중심으로 입력/출력 포트를 인터페이스로 정의하고 어댑터로 구현
* **Layered Architecture** — 전통적인 Controller → Service → Repository 수직 계층 구조
* **No explicit architecture** — 명시적인 구조 없이 필요에 따라 패키지 분리

## Decision Outcome

Chosen option: "Hexagonal Architecture (Ports & Adapters)", because 도메인 로직이 외부 의존성으로부터 완전히 격리되어 SQLite→MariaDB, 파일큐→외부큐 등 어댑터 교체가 용이하고, 포트 인터페이스 기반으로 도메인 로직을 독립적으로 테스트할 수 있다.

## Consequences

* Good, because 어댑터 교체 용이(SQLite→MariaDB, 파일큐→외부큐), 도메인 로직 독립 테스트 가능하며 외부 변화로부터 비즈니스 로직을 보호한다.
* Bad, because 인터페이스 정의 오버헤드가 발생하고 디렉토리 구조 복잡도가 증가한다.

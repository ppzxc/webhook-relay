---
status: accepted
date: 2026-03-01
---

# ADR-0019: MariaDB 스토리지 어댑터 추가

## Context and Problem Statement

SQLite는 단일 파일 기반으로 소규모 배포에 적합하지만, 메시지 볼륨이 크거나 외부 DB를 사용하는 환경에서는 MariaDB 같은 서버 DB가 필요하다. 헥사고날 아키텍처의 포트/어댑터 구조 덕분에 스토리지 레이어를 교체 가능한 구조가 이미 갖춰져 있었다.

## Considered Options

* **MariaDB 어댑터 추가** — MariaDB/MySQL 호환 드라이버를 사용하는 새 스토리지 어댑터 구현
* **PostgreSQL 어댑터 추가** — PostgreSQL 드라이버를 사용하는 어댑터 구현
* **SQLite만 지원** — 단일 스토리지로 SQLite를 유지하고 스케일링은 다른 방법으로 해결

## Decision Outcome

Chosen option: "MariaDB 어댑터 추가", because 대용량 메시지 처리 환경을 지원하면서도 기존 MariaDB 인프라를 활용할 수 있고, 스토리지 팩토리 패턴으로 runtime에 SQLite↔MariaDB 전환이 가능하기 때문이다.

## Consequences

* Good, because 대용량 메시지 처리 환경 지원, 기존 MariaDB 인프라 활용 가능, 스토리지 팩토리 패턴으로 runtime에 SQLite↔MariaDB 전환 가능
* Bad, because MariaDB 서버 운영 부담, 두 스토리지 어댑터 동기화 유지 필요 (schema 변경 시 양쪽 수정)

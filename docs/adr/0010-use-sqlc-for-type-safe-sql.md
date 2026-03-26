---
status: accepted
date: 2026-01-01
---

# ADR-0010: sqlc로 타입 안전 SQL 코드 생성

## Context and Problem Statement

SQLite와 MariaDB 두 스토리지를 지원해야 한다. 원시 SQL 쿼리를 직접 작성하면 타입 안전성이 없고 오류 발생 시 런타임에서야 발견된다.

## Considered Options

* **sqlc (SQL → Go 코드 생성)** — `query.sql`과 `schema.sql`을 작성하면 타입 안전한 Go 코드를 자동 생성
* **GORM (ORM)** — 구조체 태그 기반 ORM으로 SQL 추상화
* **원시 database/sql** — Go 표준 라이브러리로 SQL 직접 작성

## Decision Outcome

Chosen option: "sqlc", because SQL이 컴파일 타임에 검증되고 생성된 Go 코드가 타입 안전하며 ORM 없이 SQL을 직접 제어할 수 있다.

## Consequences

* Good, because SQL이 컴파일 타임에 검증됨, 생성된 Go 코드가 타입 안전, ORM 없이 SQL 직접 제어 가능
* Bad, because query.sql/schema.sql 변경 시 sqlc generate 재실행 필요, 생성 코드(db/ 디렉토리)를 직접 편집 불가

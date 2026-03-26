---
status: accepted
date: 2026-02-15
---

# ADR-0017: CGO-free SQLite로 마이그레이션 (mattn → modernc)

> Supersedes: mattn/go-sqlite3 사용 결정 (v0.1.0)

## Context and Problem Statement

초기에 mattn/go-sqlite3를 사용했으나 CGO 의존성으로 인해 크로스 컴파일(linux/arm64, darwin, windows)이 복잡해졌다. 완전한 정적 바이너리 배포를 위해 CGO를 제거해야 했다. CI 파이프라인에서 플랫폼별 C 툴체인을 별도로 설치해야 하는 부담도 컸다.

## Considered Options

* **modernc.org/sqlite (CGO-free 순수 Go 구현)** — CGO 없이 동작하는 순수 Go SQLite 구현체 사용
* **mattn/go-sqlite3 유지 + CGO 크로스컴파일 설정** — 플랫폼별 cross-compilation 툴체인을 CI에 구성하여 유지
* **SQLite 포기 + PostgreSQL만 지원** — SQLite를 제거하고 PostgreSQL 단일 스토리지로 전환

## Decision Outcome

Chosen option: "modernc.org/sqlite", because CGO_ENABLED=0으로 6개 플랫폼 크로스 컴파일을 단일 Go 툴체인으로 처리할 수 있어 빌드 복잡도를 크게 낮출 수 있기 때문이다.

## Consequences

* Good, because CGO_ENABLED=0으로 6개 플랫폼 크로스 컴파일 가능, 정적 바이너리, Docker 이미지 단순화
* Bad, because mattn 대비 성능이 미세하게 낮을 수 있음, 일부 SQLite extension 미지원 가능성

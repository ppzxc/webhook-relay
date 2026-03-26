# Architecture Decision Records

이 디렉토리는 relaybox 프로젝트의 아키텍처 결정사항을 [MADR 4.0.0](https://adr.github.io/madr/) 형식으로 기록합니다.

## 목록

| ADR | 제목 | 상태 | 날짜 |
|-----|------|------|------|
| [0001](0001-record-architecture-decisions.md) | ADR로 아키텍처 결정 기록 | accepted | 2026-03-26 |
| [0002](0002-use-hexagonal-architecture.md) | 헥사고날 아키텍처 채택 | accepted | 2026-01-01 |
| [0003](0003-use-manual-dependency-injection.md) | 수동 의존성 주입 (DI 프레임워크 없음) | accepted | 2026-01-01 |
| [0004](0004-use-chi-http-router.md) | chi를 HTTP 라우터로 선택 | accepted | 2026-01-01 |
| [0005](0005-use-ulid-for-message-ids.md) | ULID를 메시지 ID로 사용 | accepted | 2026-01-01 |
| [0006](0006-use-file-based-message-queue.md) | 파일 기반 메시지 큐 | accepted | 2026-01-01 |
| [0007](0007-use-ack-nack-queue-semantics.md) | Ack/Nack 큐 소비 패턴 | accepted | 2026-01-01 |
| [0008](0008-use-string-enums-for-domain-types.md) | string 기반 도메인 열거형 | accepted | 2026-01-01 |
| [0009](0009-use-explicit-message-status-machine.md) | 명시적 메시지 상태 기계 | accepted | 2026-01-01 |
| [0010](0010-use-sqlc-for-type-safe-sql.md) | sqlc로 타입 안전 SQL 코드 생성 | accepted | 2026-01-01 |
| [0011](0011-use-yaml-config-with-hot-reload.md) | Viper YAML 설정 + 핫 리로드 | accepted | 2026-01-01 |
| [0012](0012-use-rfc7807-error-responses.md) | RFC 7807 Problem Details 에러 응답 형식 | accepted | 2026-01-01 |
| [0013](0013-use-api-version-header.md) | URL 버저닝 대신 X-API-Version 응답 헤더 사용 | accepted | 2026-01-01 |
| [0014](0014-add-dual-expression-engines.md) | CEL + Expr 듀얼 표현식 엔진 | accepted | 2026-02-01 |
| [0015](0015-add-multi-protocol-input.md) | 다중 프로토콜 입력 지원 (HTTP, WebSocket, TCP) | accepted | 2026-02-01 |
| [0016](0016-add-parser-pipeline.md) | 파서 파이프라인 (graceful degradation) | accepted | 2026-02-01 |
| [0017](0017-migrate-to-cgo-free-sqlite.md) | CGO-free SQLite로 마이그레이션 (mattn → modernc) | accepted | 2026-02-15 |
| [0018](0018-use-input-id-as-routing-key.md) | InputType 열거형 제거, input ID를 라우팅 키로 사용 | accepted | 2026-02-20 |
| [0019](0019-add-mariadb-storage-adapter.md) | MariaDB 스토리지 어댑터 추가 | accepted | 2026-03-01 |
| [0020](0020-use-dot-notation-template-keys.md) | dot-notation 템플릿 키로 중첩 JSON 생성 | accepted | 2026-02-10 |

## 새 ADR 추가

새로운 아키텍처 결정이 있을 때:

1. 다음 번호로 파일 생성: `NNNN-title-with-dashes.md`
2. 아래 템플릿 사용:

```markdown
---
status: accepted
date: YYYY-MM-DD
---

# ADR-NNNN: 제목

## Context and Problem Statement

왜 이 결정이 필요했는지 2-3문장.

## Considered Options

* **Option A** — 한 줄 설명
* **Option B** — 한 줄 설명

## Decision Outcome

Chosen option: "Option A", because {핵심 이유}.

## Consequences

* Good, because {긍정적 영향}
* Bad, because {부정적 영향 또는 트레이드오프}
```

3. 이 README의 목록에 추가

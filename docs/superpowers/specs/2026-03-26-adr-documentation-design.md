# ADR Documentation Design Spec

## Context

relaybox 프로젝트에는 약 20개의 아키텍처 결정사항이 git 히스토리에 암묵적으로 존재하지만, 명시적 ADR 문서는 없다. 코드만 봐서는 "왜" 그런 선택을 했는지 알 수 없어, Claude Code가 향후 변경 시 맥락을 놓칠 수 있다. MADR 형식으로 `docs/adr/`에 정리하여 아키텍처 의사결정의 이유와 트레이드오프를 기록한다.

## Design

### 저장 경로

`docs/adr/` 디렉토리. `README.md` 인덱스 파일 포함.

### 템플릿

```markdown
---
status: accepted
date: YYYY-MM-DD
---

# ADR-NNNN: 짧은 제목

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

- `status`: `accepted` | `deprecated` | `superseded by ADR-NNNN`
- `date`: git 커밋 기준 결정 시점
- 한글 작성, 기술 용어는 원문 유지
- MADR 4.0.0 기반, Claude Code 가독성 최적화

### ADR 목록 (20개)

| # | 파일명 | 제목 | 근거 커밋 |
|---|--------|------|-----------|
| 0001 | `0001-record-architecture-decisions.md` | ADR로 아키텍처 결정 기록 | 신규 (self-referential) |
| 0002 | `0002-use-hexagonal-architecture.md` | 헥사고날 아키텍처 채택 | `377d90d`, `e202569` |
| 0003 | `0003-use-manual-dependency-injection.md` | 수동 DI (프레임워크 없음) | `7985b01` |
| 0004 | `0004-use-chi-http-router.md` | chi를 HTTP 라우터로 선택 | `7985b01` |
| 0005 | `0005-use-ulid-for-message-ids.md` | ULID를 메시지 ID로 사용 | `7985b01` |
| 0006 | `0006-use-file-based-message-queue.md` | 파일 기반 메시지 큐 | `7985b01` |
| 0007 | `0007-use-ack-nack-queue-semantics.md` | Ack/Nack 큐 소비 패턴 | `7985b01` |
| 0008 | `0008-use-string-enums-for-domain-types.md` | string 기반 도메인 열거형 | `7985b01` |
| 0009 | `0009-use-explicit-message-status-machine.md` | 명시적 메시지 상태 기계 | `7985b01` |
| 0010 | `0010-use-sqlc-for-type-safe-sql.md` | sqlc로 타입 안전 SQL 생성 | `7985b01` |
| 0011 | `0011-use-yaml-config-with-hot-reload.md` | Viper YAML 설정 + 핫 리로드 | `7985b01` |
| 0012 | `0012-use-rfc7807-error-responses.md` | RFC 7807 에러 응답 포맷 | `7985b01` |
| 0013 | `0013-use-api-version-header.md` | URL 버저닝 대신 X-API-Version 헤더 | `7985b01` |
| 0014 | `0014-add-dual-expression-engines.md` | CEL + Expr 듀얼 표현식 엔진 | `3658433` |
| 0015 | `0015-add-multi-protocol-input.md` | 다중 프로토콜 입력 (HTTP+WS+TCP) | `3658433` |
| 0016 | `0016-add-parser-pipeline.md` | 파서 파이프라인 (graceful degradation) | `3658433` |
| 0017 | `0017-migrate-to-cgo-free-sqlite.md` | CGO-free SQLite로 마이그레이션 | `43677dd` |
| 0018 | `0018-use-input-id-as-routing-key.md` | InputType 제거, input ID를 라우팅 키로 | `ad946e8` |
| 0019 | `0019-add-mariadb-storage-adapter.md` | MariaDB 스토리지 어댑터 추가 | `00cf726` |
| 0020 | `0020-use-dot-notation-template-keys.md` | dot-notation 템플릿 키로 중첩 JSON 생성 | `e3538a7` |

### README.md 인덱스

`docs/adr/README.md`에 전체 ADR 목록을 테이블로 정리. 상태(accepted/deprecated/superseded)와 한줄 요약 포함.

### CLAUDE.md 업데이트

`CLAUDE.md`의 Architecture 섹션에 다음 안내 추가:

```
### ADR (Architecture Decision Records)
아키텍처 결정사항은 `docs/adr/`에 MADR 형식으로 기록. 변경 전 관련 ADR을 확인하고, 새로운 아키텍처 결정 시 ADR을 추가할 것.
```

## 산출물

1. `docs/adr/README.md` — 인덱스
2. `docs/adr/0001-record-architecture-decisions.md` ~ `0020-use-dot-notation-template-keys.md` — ADR 20개
3. `CLAUDE.md` 업데이트 — ADR 참조 안내

## Verification

1. 모든 20개 `.md` 파일이 `docs/adr/`에 존재하는지 확인
2. 각 파일의 YAML frontmatter (`status`, `date`)가 올바른지 확인
3. README.md 인덱스가 전체 목록과 일치하는지 확인
4. CLAUDE.md에 ADR 참조가 추가되었는지 확인

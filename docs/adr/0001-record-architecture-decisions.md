---
status: accepted
date: 2026-03-26
---

# ADR-0001: ADR로 아키텍처 결정 기록

## Context and Problem Statement

결정사항이 코드에만 암묵적으로 존재하고 "왜" 그 선택을 했는지 알기 어렵다. Claude Code 같은 AI 도구가 컨텍스트 없이 코드를 읽을 때 결정 배경을 파악하지 못한다. 이후 유지보수 시 동일한 논의를 반복하게 될 위험이 있다.

## Considered Options

* **MADR 형식** — Markdown Any Decision Records, 구조화된 섹션과 front matter를 갖춘 경량 형식
* **Lightweight format (단순 텍스트)** — 별도 구조 없이 자유 형식 텍스트로 기록
* **기록하지 않음** — 결정사항을 코드와 커밋 메시지에만 남김

## Decision Outcome

Chosen option: "MADR 형식", because 구조화된 섹션(Context, Options, Decision, Consequences)이 Claude Code 가독성에 최적화되어 있고, AI 도구와 팀원 모두 결정 배경을 빠르게 파악할 수 있다.

## Consequences

* Good, because AI 도구와 팀원이 결정 배경을 빠르게 파악 가능하고 반복적인 논의를 줄일 수 있다.
* Bad, because 새로운 아키텍처 결정 시 ADR 작성 오버헤드가 발생한다.

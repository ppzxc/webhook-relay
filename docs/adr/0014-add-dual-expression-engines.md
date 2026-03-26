---
status: accepted
date: 2026-02-01
---

# ADR-0014: CEL + Expr 듀얼 표현식 엔진

## Context and Problem Statement

v0.2.0에서 메시지 필터링/매핑/라우팅에 표현식 언어가 필요해졌다. 단일 엔진을 강제하면 사용자 선택권이 없고 각 엔진마다 강점이 다르다. CEL(Common Expression Language)은 강타입 복잡 표현식에 적합하고 Expr은 Go-native 문법으로 간결한 표현식을 제공한다.

## Considered Options

* **CEL만 사용** — Google Common Expression Language 단일 엔진으로 모든 표현식을 처리
* **Expr만 사용** — antonmedv/expr 단일 엔진으로 Go-native 문법의 표현식을 처리
* **플러그인 레지스트리 (CEL + Expr 모두)** — 표현식 엔진을 레지스트리로 관리하여 사용자가 입력/출력별로 엔진을 선택

## Decision Outcome

Chosen option: "플러그인 레지스트리 (CEL + Expr 모두)", because 단일 엔진 강제는 특정 사용 사례에서 표현력 한계를 초래하며, 레지스트리 패턴은 헥사고날 아키텍처와 일관되게 향후 추가 엔진 확장도 열어두기 때문이다.

## Consequences

* Good, because 사용자가 입력/출력별로 엔진을 선택할 수 있어 표현식 유연성이 높아짐
* Good, because CEL은 강타입 복잡 표현식에, Expr은 Go-native 간결한 표현식에 각각 적합하게 사용 가능
* Good, because 레지스트리 패턴으로 향후 새로운 표현식 엔진을 추가하기 용이함
* Bad, because 두 엔진의 문법 차이로 rules 파일에서 혼용 시 사용자 혼란이 발생할 수 있음
* Bad, because 두 엔진 의존성이 모두 포함되어 바이너리 크기가 증가함

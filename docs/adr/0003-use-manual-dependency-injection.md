---
status: accepted
date: 2026-01-01
---

# ADR-0003: 수동 의존성 주입 (DI 프레임워크 없음)

## Context and Problem Statement

여러 어댑터와 서비스를 조립해야 한다. Go 생태계에는 Wire, Fx 같은 DI 프레임워크가 있지만 각각 코드 생성과 런타임 리플렉션이라는 트레이드오프를 갖는다. 프로젝트 초기에는 의존성 그래프가 단순하므로 프레임워크 도입 비용 대비 효용을 검토해야 한다.

## Considered Options

* **수동 DI (cmd/server/main.go)** — `runServer()` 함수에서 모든 어댑터를 직접 생성하고 주입
* **Wire (코드 생성 기반)** — 컴파일 타임에 DI 코드를 자동 생성하는 Google Wire
* **Fx (런타임 리플렉션 기반)** — 런타임에 의존성을 해소하는 Uber Fx

## Decision Outcome

Chosen option: "수동 DI (cmd/server/main.go)", because 외부 의존성 없이 조립 순서와 의존관계가 `cmd/server/main.go`에 명시적으로 보이며, 현재 의존성 그래프 규모에서 프레임워크 도입 비용이 이점을 초과한다.

## Consequences

* Good, because 외부 의존성 없음, 조립 순서와 의존관계가 코드에 명시적으로 보여 디버깅과 온보딩이 용이하다.
* Bad, because 의존성 추가 시 `main.go`를 직접 수정해야 하며 의존성 그래프가 복잡해질수록 관리 부담이 증가한다.

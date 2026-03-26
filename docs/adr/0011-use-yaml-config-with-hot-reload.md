---
status: accepted
date: 2026-01-01
---

# ADR-0011: Viper YAML 설정 + 핫 리로드

## Context and Problem Statement

서버 재시작 없이 라우팅 규칙(rules)과 출력 설정(outputs)을 변경할 수 있어야 한다. 설정 파일 형식은 사람이 읽고 편집하기 쉬워야 한다. 운영 중에 규칙을 수정해야 하는 경우 다운타임을 최소화하는 것이 중요하다.

## Considered Options

* **Viper + YAML + fsnotify WatchConfig()** — Viper 라이브러리로 YAML 파일을 로드하고 fsnotify 기반 WatchConfig()로 변경 감지 후 핫 리로드
* **환경변수만 사용** — 모든 설정을 환경변수로 주입, 재시작 없이는 변경 불가
* **설정 파일 + 수동 재시작** — YAML 파일은 유지하되 핫 리로드 없이 변경 시 서버를 재시작

## Decision Outcome

Chosen option: "Viper + YAML + fsnotify WatchConfig()", because rules/outputs는 운영 중 빈번히 변경될 수 있어 재시작 없는 반영이 필수이고, Viper는 기본값 설정·환경변수 오버라이드·강력한 언마샬링을 동시에 지원하기 때문이다.

## Consequences

* Good, because rules/outputs 변경 시 서버 재시작이 불필요하여 다운타임이 없음
* Good, because YAML 형식은 가독성이 높아 사람이 직접 편집하기 쉬움
* Good, because Viper의 기본값 설정, 환경변수 오버라이드, 타입 검증 기능을 활용할 수 있음
* Bad, because 핫 리로드는 rules/outputs만 지원하며 서버 바인딩 주소나 스토리지 경로 등 서버·스토리지 설정 변경은 여전히 재시작이 필요함
* Bad, because 설정 파일 오류(잘못된 YAML, 잘못된 CEL 표현식 등)가 런타임에 감지되어 운영 중 영향을 줄 수 있음

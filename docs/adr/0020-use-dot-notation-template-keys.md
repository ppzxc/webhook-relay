---
status: accepted
date: 2026-02-10
---

# ADR-0020: dot-notation 템플릿 키로 중첩 JSON 생성

## Context and Problem Statement

웹훅 페이로드를 렌더링할 때 Slack/Discord 같은 서비스는 중첩 JSON 구조를 요구한다. 설정 파일에서 `"content.text": "{{ .message }}"` 형태로 중첩 구조를 표현하고 싶다. 사용자가 직접 중첩 JSON을 작성하게 하면 설정 파일이 복잡해지고 실수가 늘어난다.

## Considered Options

* **dot-notation 키 자동 변환** — `"a.b.c"` 형태의 키를 `{"a":{"b":{"c":val}}}` 중첩 구조로 자동 변환
* **사용자가 직접 중첩 JSON 작성** — 설정 파일에 완전한 중첩 JSON을 직접 기술
* **Go text/template으로 전체 JSON 작성** — payload 전체를 Go text/template으로 렌더링

## Decision Outcome

Chosen option: "dot-notation 키 자동 변환", because Slack/Discord webhook 포맷을 간결하게 표현할 수 있어 설정 파일 가독성이 높아지고, 사용자의 실수를 줄일 수 있기 때문이다.

## Consequences

* Good, because Slack/Discord webhook 포맷을 간결하게 표현 가능, 설정 파일 가독성 향상
* Bad, because 점(.)이 포함된 키 이름을 리터럴로 사용할 수 없음, 변환 로직이 추가 복잡도 야기

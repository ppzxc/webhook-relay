---
status: accepted
date: 2026-02-01
---

# ADR-0016: 파서 파이프라인 (graceful degradation)

## Context and Problem Statement

입력 메시지가 JSON, URL-encoded form, XML, logfmt 등 다양한 형식으로 올 수 있다. CEL/Expr 표현식으로 필드를 참조하려면 파싱된 구조체가 필요하다. 단일 포맷만 지원하면 다양한 소스에서 오는 웹훅을 처리할 수 없어 범용 릴레이 허브로서의 역할을 수행하기 어렵다.

## Considered Options

* **파서 파이프라인 (graceful degradation)** — JSON, Form, XML, Logfmt, Regex 등 다수의 파서를 순서대로 시도하고, 모두 실패하면 raw payload로 폴백
* **JSON만 지원** — JSON 파싱만 구현하여 단순하게 유지
* **파싱 없이 raw payload만** — 파싱 없이 raw 문자열 그대로 CEL 표현식에 노출

## Decision Outcome

Chosen option: "파서 파이프라인 (graceful degradation)", because 다양한 소스(Slack, GitHub, Beszel 등)의 웹훅 형식을 유연하게 지원하면서도, 파싱 실패 시 raw payload로 폴백하여 메시지 유실을 방지할 수 있기 때문이다.

## Consequences

* Good, because JSON/Form/XML/Logfmt/Regex 등 다양한 소스 지원, 파싱 실패 시 raw payload로 폴백하여 메시지 유실 방지
* Bad, because 각 파서 어댑터 유지보수 필요, 파싱 결과(ParsedData)는 DB에 저장되지 않고 처리 중에만 사용

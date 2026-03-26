---
status: accepted
date: 2026-02-01
---

# ADR-0015: 다중 프로토콜 입력 지원 (HTTP, WebSocket, TCP)

## Context and Problem Statement

다양한 소스(Beszel, Grafana 등)에서 메시지를 수신해야 한다. 일부 소스는 HTTP POST를 지원하지 않고 WebSocket이나 TCP 스트림을 사용한다. 단일 프로토콜만 지원하면 수신 불가능한 소스가 생겨 범용 webhook relay 허브로서의 역할을 다하지 못한다.

## Considered Options

* **HTTP만 지원** — POST /inputs/{inputId}/messages 엔드포인트만으로 모든 메시지를 수신
* **HTTP + WebSocket** — HTTP와 함께 gorilla/websocket 기반 WebSocket 인바운드 핸들러 추가
* **HTTP + WebSocket + TCP** — 세 프로토콜 모두 지원하여 TCP 스트림 소스까지 커버

## Decision Outcome

Chosen option: "HTTP + WebSocket + TCP (세 프로토콜 모두)", because Beszel 같은 모니터링 도구는 TCP 또는 WebSocket 출력만 지원하며, 범용 relay 허브를 목표로 하는 relaybox는 소스의 프로토콜 제약에 의존하지 않아야 하기 때문이다.

## Consequences

* Good, because Beszel 같은 모니터링 도구의 TCP/WebSocket 출력을 직접 수신할 수 있음
* Good, because 헥사고날 아키텍처의 driving adapter 패턴으로 새 프로토콜 어댑터를 독립적으로 추가하기 용이함
* Bad, because HTTP, WebSocket, TCP 세 개의 driving adapter를 각각 유지보수해야 하므로 운영 복잡도가 증가함
* Bad, because TCP는 프레임 경계가 없어 직접 delimiter 파싱 구현이 필요하며 엣지 케이스 처리 부담이 있음

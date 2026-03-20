# webhook-relay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** beszel, dozzle 등 모니터링 앱의 알람을 HTTP 웹훅/WebSocket으로 수신해 YAML 템플릿 변환 후 외부 채널로 전달하는 헥사고날 아키텍처 기반 Go 릴레이 앱을 구현한다.

**Architecture:** Hexagonal Architecture — domain(의존성 0) → application/port(인터페이스) ← application/service(유스케이스) ↔ adapter(구현체). DI는 cmd/server/main.go에서만 조립한다. 빌드 진입점은 `cmd/server/main.go` 단일 파일 (루트 main.go 없음).

**Tech Stack:** Go 1.25, cobra, viper, chi, gorilla/websocket, sqlc + mattn/go-sqlite3, slog, ULID

---

## 파일 구조 맵

```
cmd/server/main.go            진입점 + Cobra + DI 조립

internal/domain/
  alert.go                   Alert 엔티티, RawPayload
  alert_status.go            AlertStatus enum (string)
  channel.go                 Channel 값 객체
  channel_type.go            ChannelType enum (string)
  route.go                   Route 매핑
  source_type.go             SourceType enum (string)
  template.go                RenderTemplate 도메인 헬퍼
  errors.go                  sentinel errors
  alert_test.go
  template_test.go

internal/application/port/input/
  receive_alert.go           ReceiveAlertUseCase (source domain.SourceType 사용)

internal/application/port/output/
  alert_repository.go
  alert_sender.go
  alert_queue.go             AckFunc + NackFunc 정의 포함
  route_config_reader.go
  sender_registry.go         Get() 반환 타입: AlertSender (named interface)

internal/application/service/
  alert_service.go
  alert_service_test.go
  delivery_worker.go
  delivery_worker_test.go

internal/config/
  config.go                  Config 구조체, Load(), Watch()
  route_config_reader.go     InMemoryRouteConfigReader
  config_test.go
  config.example.yaml

internal/adapter/output/sqlite/
  schema.sql
  query.sql
  sqlc.yaml
  db/                        sqlc 생성 파일
  repository.go              AlertRepository 구현
  repository_test.go

internal/adapter/output/filequeue/
  queue.go                   AlertQueue 구현 (output.AckFunc/NackFunc 사용)
  queue_test.go

internal/adapter/output/webhook/
  sender.go                  AlertSender 구현
  registry.go                SenderRegistry 구현 (output.AlertSender 반환)
  sender_test.go

internal/adapter/input/http/
  router.go                  chi 라우터, X-API-Version 미들웨어
  middleware.go              인증, RFC 7807 에러
  handler.go                 HTTP 핸들러
  source_resolver.go         URL sourceID → domain.SourceType 변환
  handler_test.go

internal/adapter/input/websocket/
  handler.go
  handler_test.go

test/e2e/
  e2e_test.go
```

---

## Task 1: 프로젝트 스캐폴딩 (구조적 변경)

> tidy-first: 이 태스크는 구조적 변경만 수행. 비즈니스 로직 없음.

**Files:**
- Modify: `go.mod`
- Create: `cmd/server/main.go` (빈 main)
- Delete: `main.go` (플레이스홀더 제거)

- [ ] **Step 1: 디렉토리 구조 생성**

```bash
mkdir -p cmd/server
mkdir -p internal/domain
mkdir -p internal/application/port/input
mkdir -p internal/application/port/output
mkdir -p internal/application/service
mkdir -p internal/adapter/input/http
mkdir -p internal/adapter/input/websocket
mkdir -p internal/adapter/output/sqlite/db
mkdir -p internal/adapter/output/filequeue
mkdir -p internal/adapter/output/webhook
mkdir -p internal/config
mkdir -p test/e2e
mkdir -p data
echo "data/" >> .gitignore
```

- [ ] **Step 2: 의존성 추가**

```bash
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest
go get github.com/go-chi/chi/v5@latest
go get github.com/gorilla/websocket@latest
go get github.com/mattn/go-sqlite3@latest
go get github.com/oklog/ulid/v2@latest
go mod tidy
```

- [ ] **Step 3: sqlc 설치**

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc version
```

Expected: 버전 출력됨

- [ ] **Step 4: 진입점 파일 생성 (빈 상태)**

```bash
cat > cmd/server/main.go << 'EOF'
package main

func main() {}
EOF
```

루트 `main.go` 삭제:

```bash
rm main.go
```

- [ ] **Step 5: 빌드 확인**

```bash
go build ./cmd/server/
```

Expected: 에러 없음

- [ ] **Step 6: 커밋 (구조적)**

```bash
git add -A
git commit -m "chore: scaffold project directory structure and dependencies"
```

---

## Task 2: 도메인 레이어 — 열거형 & 엔티티 & 에러

**Files:**
- Create: `internal/domain/errors.go`
- Create: `internal/domain/alert_status.go`
- Create: `internal/domain/channel_type.go`
- Create: `internal/domain/source_type.go`
- Create: `internal/domain/alert.go`
- Create: `internal/domain/channel.go`
- Create: `internal/domain/route.go`
- Test: `internal/domain/alert_test.go`

- [ ] **Step 1: 테스트 작성**

`internal/domain/alert_test.go`:

```go
package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"webhook-relay/internal/domain"
)

func TestAlertStatus_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		input domain.AlertStatus
		want  bool
	}{
		{"pending", domain.AlertStatusPending, true},
		{"delivered", domain.AlertStatusDelivered, true},
		{"failed", domain.AlertStatusFailed, true},
		{"unknown", domain.AlertStatus("UNKNOWN"), false},
		{"empty", domain.AlertStatus(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlertStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from domain.AlertStatus
		to   domain.AlertStatus
		want bool
	}{
		{domain.AlertStatusPending, domain.AlertStatusDelivered, true},
		{domain.AlertStatusPending, domain.AlertStatusFailed, true},
		{domain.AlertStatusFailed, domain.AlertStatusPending, true},
		{domain.AlertStatusDelivered, domain.AlertStatusPending, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
				t.Errorf("CanTransitionTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRawPayload_MarshalJSON(t *testing.T) {
	payload := domain.RawPayload(`{"level":"critical"}`)
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	var result domain.RawPayload
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if string(result) != string(payload) {
		t.Errorf("got %s, want %s", result, payload)
	}
}

func TestAlert_JSON_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	a := domain.Alert{
		ID:        "01J...",
		Version:   1,
		Source:    domain.SourceTypeBeszel,
		Payload:   domain.RawPayload(`{"host":"server1"}`),
		CreatedAt: now,
		Status:    domain.AlertStatusPending,
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got domain.Alert
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != a.ID || got.Status != a.Status || string(got.Payload) != string(a.Payload) {
		t.Errorf("round-trip mismatch")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/domain/...
```

Expected: FAIL — 패키지 없음

- [ ] **Step 3: 에러 정의**

`internal/domain/errors.go`:

```go
package domain

import "errors"

var (
	ErrSourceNotFound    = errors.New("source not found")
	ErrInvalidToken      = errors.New("invalid token")
	ErrAlertNotFound     = errors.New("alert not found")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrSenderNotFound    = errors.New("sender not registered for channel type")
)
```

- [ ] **Step 4: 열거형 구현**

`internal/domain/alert_status.go`:

```go
package domain

type AlertStatus string

const (
	AlertStatusPending   AlertStatus = "PENDING"
	AlertStatusDelivered AlertStatus = "DELIVERED"
	AlertStatusFailed    AlertStatus = "FAILED"
)

func (s AlertStatus) IsValid() bool {
	switch s {
	case AlertStatusPending, AlertStatusDelivered, AlertStatusFailed:
		return true
	}
	return false
}

func (s AlertStatus) CanTransitionTo(next AlertStatus) bool {
	switch s {
	case AlertStatusPending:
		return next == AlertStatusDelivered || next == AlertStatusFailed
	case AlertStatusFailed:
		return next == AlertStatusPending
	}
	return false
}
```

`internal/domain/channel_type.go`:

```go
package domain

type ChannelType string

const (
	ChannelTypeWebhook  ChannelType = "WEBHOOK"
	ChannelTypeSlack    ChannelType = "SLACK"
	ChannelTypeDiscord  ChannelType = "DISCORD"
)

func (c ChannelType) IsValid() bool {
	switch c {
	case ChannelTypeWebhook, ChannelTypeSlack, ChannelTypeDiscord:
		return true
	}
	return false
}
```

`internal/domain/source_type.go`:

```go
package domain

type SourceType string

const (
	SourceTypeBeszel  SourceType = "BESZEL"
	SourceTypeDozzle  SourceType = "DOZZLE"
	SourceTypeGeneric SourceType = "GENERIC"
)

func (s SourceType) IsValid() bool {
	switch s {
	case SourceTypeBeszel, SourceTypeDozzle, SourceTypeGeneric:
		return true
	}
	return false
}
```

- [ ] **Step 5: Alert 엔티티 구현**

`internal/domain/alert.go`:

```go
package domain

import (
	"encoding/json"
	"time"
)

type RawPayload []byte

func (r RawPayload) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return r, nil
}

func (r *RawPayload) UnmarshalJSON(b []byte) error {
	*r = make(RawPayload, len(b))
	copy(*r, b)
	return nil
}

type Alert struct {
	ID            string      `json:"id"`
	Version       int         `json:"version"`
	Source        SourceType  `json:"source"`
	Payload       RawPayload  `json:"payload"`
	CreatedAt     time.Time   `json:"createdAt"`
	Status        AlertStatus `json:"status"`
	RetryCount    int         `json:"retryCount"`
	LastAttemptAt *time.Time  `json:"lastAttemptAt,omitempty"`
}

func (a Alert) MarshalJSON() ([]byte, error) {
	type Alias struct {
		ID            string     `json:"id"`
		Version       int        `json:"version"`
		Source        string     `json:"source"`
		Payload       RawPayload `json:"payload"`
		CreatedAt     time.Time  `json:"createdAt"`
		Status        string     `json:"status"`
		RetryCount    int        `json:"retryCount"`
		LastAttemptAt *time.Time `json:"lastAttemptAt,omitempty"`
	}
	return json.Marshal(Alias{
		ID: a.ID, Version: a.Version, Source: string(a.Source),
		Payload: a.Payload, CreatedAt: a.CreatedAt, Status: string(a.Status),
		RetryCount: a.RetryCount, LastAttemptAt: a.LastAttemptAt,
	})
}
```

`internal/domain/channel.go`:

```go
package domain

type Channel struct {
	ID            string
	Type          ChannelType
	URL           string
	Template      string
	Secret        string
	RetryCount    int
	RetryDelayMs  int
	SkipTLSVerify bool
}
```

`internal/domain/route.go`:

```go
package domain

type Route struct {
	SourceID   string
	ChannelIDs []string
}
```

- [ ] **Step 6: 테스트 통과 확인**

```bash
go test ./internal/domain/... -v
```

Expected: PASS

- [ ] **Step 7: 커밋**

```bash
git add internal/domain/
git commit -m "feat(domain): add entities, enums, and sentinel errors"
```

---

## Task 3: 도메인 — 템플릿 렌더링

**Files:**
- Create: `internal/domain/template.go`
- Test: `internal/domain/template_test.go`

- [ ] **Step 1: 테스트 작성**

`internal/domain/template_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"webhook-relay/internal/domain"
)

func TestRenderTemplate(t *testing.T) {
	alert := domain.Alert{
		ID:        "abc123",
		Source:    domain.SourceTypeBeszel,
		Payload:   domain.RawPayload(`{"host":"server1"}`),
		CreatedAt: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Status:    domain.AlertStatusPending,
	}

	tests := []struct {
		name    string
		tmpl    string
		want    string
		wantErr bool
	}{
		{
			name: "source and id",
			tmpl: `{"text":"{{ .Source }}: {{ .ID }}"}`,
			want: `{"text":"BESZEL: abc123"}`,
		},
		{
			name:    "invalid syntax",
			tmpl:    `{{ .Source`,
			wantErr: true,
		},
		{
			name: "payload field",
			tmpl: `{{ .Payload }}`,
			want: `{"host":"server1"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.RenderTemplate(tt.tmpl, alert)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && string(got) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	if err := domain.ValidateTemplate(`{{ .Source }}`); err != nil {
		t.Errorf("valid template failed: %v", err)
	}
	if err := domain.ValidateTemplate(`{{ .Source`); err == nil {
		t.Error("invalid template should return error")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/domain/... -run TestRenderTemplate
```

Expected: FAIL

- [ ] **Step 3: 구현**

`internal/domain/template.go`:

```go
package domain

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

type TemplateData struct {
	ID        string
	Source    string
	Payload   string
	CreatedAt time.Time
}

func RenderTemplate(tmpl string, alert Alert) ([]byte, error) {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	data := TemplateData{
		ID:        alert.ID,
		Source:    string(alert.Source),
		Payload:   string(alert.Payload),
		CreatedAt: alert.CreatedAt,
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

func ValidateTemplate(tmpl string) error {
	if _, err := template.New("").Parse(tmpl); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: 전체 도메인 테스트 통과 확인**

```bash
go test ./internal/domain/... -v
```

Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/domain/template.go internal/domain/template_test.go
git commit -m "feat(domain): add template rendering helper"
```

---

## Task 4: Application Ports (인터페이스 정의)

**Files:**
- Create: `internal/application/port/input/receive_alert.go`
- Create: `internal/application/port/output/alert_repository.go`
- Create: `internal/application/port/output/alert_sender.go`
- Create: `internal/application/port/output/alert_queue.go` ← AckFunc/NackFunc 정의
- Create: `internal/application/port/output/route_config_reader.go`
- Create: `internal/application/port/output/sender_registry.go`

인터페이스 전용 — 테스트 없음, 빌드 확인만.

- [ ] **Step 1: 인바운드 포트**

`internal/application/port/input/receive_alert.go`:

```go
package input

import (
	"context"

	"webhook-relay/internal/domain"
)

// ReceiveAlertUseCase 알람 수신 유스케이스.
// source는 반드시 domain.SourceType 값 (예: "BESZEL")으로 전달된다.
// 성공 시 생성된 alert ID를 반환한다.
type ReceiveAlertUseCase interface {
	Receive(ctx context.Context, source domain.SourceType, payload []byte) (string, error)
}
```

- [ ] **Step 2: 아웃바운드 포트**

`internal/application/port/output/alert_repository.go`:

```go
package output

import (
	"context"
	"time"

	"webhook-relay/internal/domain"
)

type AlertRepository interface {
	Save(ctx context.Context, alert domain.Alert) error
	UpdateDeliveryState(ctx context.Context, id string, status domain.AlertStatus, retryCount int, lastAttemptAt time.Time) error
	FindByID(ctx context.Context, id string) (domain.Alert, error)
	FindBySource(ctx context.Context, sourceID string, limit, offset int) ([]domain.Alert, error)
}
```

`internal/application/port/output/alert_sender.go`:

```go
package output

import (
	"context"

	"webhook-relay/internal/domain"
)

type AlertSender interface {
	Send(ctx context.Context, channel domain.Channel, alert domain.Alert) error
}
```

`internal/application/port/output/alert_queue.go`:

```go
package output

import (
	"context"

	"webhook-relay/internal/domain"
)

// AckFunc 전달 성공 후 호출 — 큐에서 영구 삭제
type AckFunc func() error

// NackFunc 전달 실패 후 호출 — 큐에 메시지 반환
type NackFunc func() error

type AlertQueue interface {
	Enqueue(ctx context.Context, alert domain.Alert) error
	Dequeue(ctx context.Context) (domain.Alert, AckFunc, NackFunc, error)
}
```

`internal/application/port/output/route_config_reader.go`:

```go
package output

import (
	"context"

	"webhook-relay/internal/domain"
)

type RouteConfigReader interface {
	GetChannels(ctx context.Context, sourceID string) ([]domain.Channel, error)
}
```

`internal/application/port/output/sender_registry.go`:

```go
package output

import "webhook-relay/internal/domain"

type SenderRegistry interface {
	// Get 은 AlertSender(named interface)를 반환한다.
	Get(channelType domain.ChannelType) (AlertSender, error)
}
```

- [ ] **Step 3: 빌드 확인**

```bash
go build ./...
```

Expected: 에러 없음

- [ ] **Step 4: 커밋**

```bash
git add internal/application/port/
git commit -m "feat(port): define all hexagonal port interfaces"
```

---

## Task 5: 설정 레이어 (Cobra + Viper)

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/route_config_reader.go`
- Create: `internal/config/config.example.yaml`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 테스트 작성**

`internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"webhook-relay/internal/config"
)

const testYAML = `
server:
  port: 9090
log:
  level: debug
  format: text
sources:
  - id: beszel
    type: BESZEL
    secret: test-secret
channels:
  - id: ops-webhook
    type: WEBHOOK
    url: https://hooks.example.com/test
    template: '{"text":"{{ .Source }}"}'
    retryCount: 3
    retryDelayMs: 500
routes:
  - sourceId: beszel
    channelIds:
      - ops-webhook
storage:
  type: SQLITE
  path: ./data/test.db
queue:
  type: FILE
  path: ./data/queue
  workerCount: 1
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, testYAML))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port = %d, want 9090", cfg.Server.Port)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].ID != "beszel" {
		t.Errorf("sources = %+v", cfg.Sources)
	}
	if len(cfg.Routes) != 1 {
		t.Errorf("routes = %+v", cfg.Routes)
	}
}

func TestLoad_InvalidTemplate(t *testing.T) {
	yaml := `
server:
  port: 8080
channels:
  - id: bad
    type: WEBHOOK
    url: https://example.com
    template: '{{ .Source'
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
}

func TestInMemoryRouteConfigReader(t *testing.T) {
	cfg, _ := config.Load(writeConfig(t, testYAML))
	reader := config.NewInMemoryRouteConfigReader(cfg)

	channels, err := reader.GetChannels(nil, "beszel")
	if err != nil {
		t.Fatalf("GetChannels error: %v", err)
	}
	if len(channels) != 1 {
		t.Errorf("got %d channels, want 1", len(channels))
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/config/...
```

Expected: FAIL

- [ ] **Step 3: Config 구조체 구현**

`internal/config/config.go`:

```go
package config

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"webhook-relay/internal/domain"
)

type ServerConfig struct {
	Port         int       `mapstructure:"port"`
	ReadTimeout  string    `mapstructure:"readTimeout"`
	WriteTimeout string    `mapstructure:"writeTimeout"`
	TLS          TLSConfig `mapstructure:"tls"`
}

type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"certFile"`
	KeyFile  string `mapstructure:"keyFile"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type SourceConfig struct {
	ID     string `mapstructure:"id"`
	Type   string `mapstructure:"type"`
	Secret string `mapstructure:"secret"`
}

type ChannelConfig struct {
	ID            string `mapstructure:"id"`
	Type          string `mapstructure:"type"`
	URL           string `mapstructure:"url"`
	Template      string `mapstructure:"template"`
	RetryCount    int    `mapstructure:"retryCount"`
	RetryDelayMs  int    `mapstructure:"retryDelayMs"`
	SkipTLSVerify bool   `mapstructure:"skipTLSVerify"`
}

type RouteConfig struct {
	SourceID   string   `mapstructure:"sourceId"`
	ChannelIDs []string `mapstructure:"channelIds"`
}

type StorageConfig struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
}

type QueueConfig struct {
	Type        string `mapstructure:"type"`
	Path        string `mapstructure:"path"`
	WorkerCount int    `mapstructure:"workerCount"`
}

type Config struct {
	Server   ServerConfig    `mapstructure:"server"`
	Log      LogConfig       `mapstructure:"log"`
	Sources  []SourceConfig  `mapstructure:"sources"`
	Channels []ChannelConfig `mapstructure:"channels"`
	Routes   []RouteConfig   `mapstructure:"routes"`
	Storage  StorageConfig   `mapstructure:"storage"`
	Queue    QueueConfig     `mapstructure:"queue"`
}

// Load 설정 파일을 읽어 Config를 반환한다. 템플릿 검증 포함.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	setDefaults(v)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return unmarshalAndValidate(v)
}

// Watch 설정 파일 변경 감지. channels/routes만 핫리로드. 유효하지 않으면 기존 유지.
func Watch(v *viper.Viper, onChange func(cfg *Config)) {
	v.WatchConfig()
	v.OnConfigChange(func(_ fsnotify.Event) {
		cfg, err := unmarshalAndValidate(v)
		if err != nil {
			return // 유효하지 않은 설정 무시
		}
		onChange(cfg)
	})
}

// NewViper path에서 viper 인스턴스를 반환한다 (Watch용).
func NewViper(path string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigFile(path)
	setDefaults(v)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return v, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.readTimeout", "30s")
	v.SetDefault("server.writeTimeout", "30s")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("queue.workerCount", 2)
}

func unmarshalAndValidate(v *viper.Viper) (*Config, error) {
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	for _, ch := range cfg.Channels {
		if ch.Template == "" {
			continue
		}
		if err := domain.ValidateTemplate(ch.Template); err != nil {
			return nil, fmt.Errorf("channel %q: %w", ch.ID, err)
		}
	}
	return &cfg, nil
}
```

- [ ] **Step 4: RouteConfigReader 구현**

`internal/config/route_config_reader.go`:

```go
package config

import (
	"context"
	"fmt"
	"sync"

	"webhook-relay/internal/domain"
)

type InMemoryRouteConfigReader struct {
	mu       sync.RWMutex
	channels map[string]domain.Channel
	routes   map[string][]string
}

func NewInMemoryRouteConfigReader(cfg *Config) *InMemoryRouteConfigReader {
	r := &InMemoryRouteConfigReader{}
	r.Update(cfg)
	return r
}

func (r *InMemoryRouteConfigReader) Update(cfg *Config) {
	channels := make(map[string]domain.Channel, len(cfg.Channels))
	for _, c := range cfg.Channels {
		channels[c.ID] = domain.Channel{
			ID: c.ID, Type: domain.ChannelType(c.Type), URL: c.URL,
			Template: c.Template, RetryCount: c.RetryCount,
			RetryDelayMs: c.RetryDelayMs, SkipTLSVerify: c.SkipTLSVerify,
		}
	}
	routes := make(map[string][]string, len(cfg.Routes))
	for _, rt := range cfg.Routes {
		routes[rt.SourceID] = rt.ChannelIDs
	}
	r.mu.Lock()
	r.channels = channels
	r.routes = routes
	r.mu.Unlock()
}

func (r *InMemoryRouteConfigReader) GetChannels(ctx context.Context, sourceID string) ([]domain.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids, ok := r.routes[sourceID]
	if !ok {
		return nil, fmt.Errorf("route for %q: %w", sourceID, domain.ErrSourceNotFound)
	}
	result := make([]domain.Channel, 0, len(ids))
	for _, id := range ids {
		if ch, ok := r.channels[id]; ok {
			result = append(result, ch)
		}
	}
	return result, nil
}
```

- [ ] **Step 5: example yaml 작성**

`internal/config/config.example.yaml`:

```yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
  tls:
    enabled: false
    certFile: ""
    keyFile: ""

log:
  level: info    # debug, info, warn, error
  format: json   # json, text

sources:
  - id: beszel
    type: BESZEL
    secret: "change-me"
  - id: dozzle
    type: DOZZLE
    secret: "change-me"

channels:
  - id: ops-webhook
    type: WEBHOOK
    url: "https://hooks.example.com/xyz"
    template: |
      {"text": "{{ .Source }}: {{ .Payload }}"}
    retryCount: 3
    retryDelayMs: 1000
    skipTLSVerify: false

routes:
  - sourceId: beszel
    channelIds: [ops-webhook]
  - sourceId: dozzle
    channelIds: [ops-webhook]

storage:
  type: SQLITE
  path: "./data/webhook-relay.db"

queue:
  type: FILE
  path: "./data/queue"
  workerCount: 2
```

- [ ] **Step 6: 테스트 통과 확인**

```bash
go test ./internal/config/... -v
```

Expected: PASS

- [ ] **Step 7: 커밋**

```bash
git add internal/config/
git commit -m "feat(config): add viper loader, hot-reload, and route config reader"
```

---

## Task 6: SQLite 어댑터 (AlertRepository)

**Files:**
- Create: `internal/adapter/output/sqlite/schema.sql`
- Create: `internal/adapter/output/sqlite/query.sql`
- Create: `internal/adapter/output/sqlite/sqlc.yaml`
- Create: `internal/adapter/output/sqlite/repository.go`
- Test: `internal/adapter/output/sqlite/repository_test.go`

- [ ] **Step 1: 테스트 작성**

`internal/adapter/output/sqlite/repository_test.go`:

```go
package sqlite_test

import (
	"context"
	"testing"
	"time"

	sqliteadapter "webhook-relay/internal/adapter/output/sqlite"
	"webhook-relay/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ interface {
	Save(context.Context, domain.Alert) error
	FindByID(context.Context, string) (domain.Alert, error)
} = (*sqliteadapter.Repository)(nil)

func newTestRepo(t *testing.T) *sqliteadapter.Repository {
	t.Helper()
	repo, err := sqliteadapter.New(":memory:")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func TestRepository_SaveAndFindByID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	alert := domain.Alert{
		ID: "test-001", Version: 1, Source: domain.SourceTypeBeszel,
		Payload: domain.RawPayload(`{"host":"srv1"}`),
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Status: domain.AlertStatusPending,
	}
	if err := repo.Save(ctx, alert); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	got, err := repo.FindByID(ctx, alert.ID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if got.ID != alert.ID || string(got.Payload) != string(alert.Payload) {
		t.Errorf("mismatch: got %+v", got)
	}
}

func TestRepository_UpdateDeliveryState(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	alert := domain.Alert{ID: "test-002", Version: 1, Source: domain.SourceTypeDozzle, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending}
	repo.Save(ctx, alert)

	now := time.Now().UTC()
	if err := repo.UpdateDeliveryState(ctx, alert.ID, domain.AlertStatusDelivered, 1, now); err != nil {
		t.Fatalf("UpdateDeliveryState() error: %v", err)
	}
	got, _ := repo.FindByID(ctx, alert.ID)
	if got.Status != domain.AlertStatusDelivered || got.RetryCount != 1 {
		t.Errorf("unexpected state: %+v", got)
	}
}

func TestRepository_FindBySource(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	for _, id := range []string{"a1", "a2", "a3"} {
		repo.Save(ctx, domain.Alert{ID: id, Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending, Version: 1})
	}
	alerts, err := repo.FindBySource(ctx, string(domain.SourceTypeBeszel), 10, 0)
	if err != nil {
		t.Fatalf("FindBySource() error: %v", err)
	}
	if len(alerts) != 3 {
		t.Errorf("got %d, want 3", len(alerts))
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/adapter/output/sqlite/...
```

Expected: FAIL

- [ ] **Step 3: sqlc 설정 파일 작성**

`internal/adapter/output/sqlite/sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: "sqlite"
    queries: "query.sql"
    schema: "schema.sql"
    gen:
      go:
        package: "db"
        out: "db"
        emit_json_tags: true
```

`internal/adapter/output/sqlite/schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS alerts (
    id              TEXT PRIMARY KEY,
    version         INTEGER NOT NULL DEFAULT 1,
    source          TEXT NOT NULL,
    payload         BLOB NOT NULL,
    created_at      DATETIME NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_attempt_at DATETIME
);
```

`internal/adapter/output/sqlite/query.sql`:

```sql
-- name: InsertAlert :exec
INSERT INTO alerts (id, version, source, payload, created_at, status, retry_count)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateDeliveryState :exec
UPDATE alerts SET status=?, retry_count=?, last_attempt_at=? WHERE id=?;

-- name: GetAlertByID :one
SELECT id, version, source, payload, created_at, status, retry_count, last_attempt_at
FROM alerts WHERE id=?;

-- name: ListAlertsBySource :many
SELECT id, version, source, payload, created_at, status, retry_count, last_attempt_at
FROM alerts WHERE source=? ORDER BY created_at DESC LIMIT ? OFFSET ?;
```

- [ ] **Step 4: sqlc generate 실행**

```bash
cd internal/adapter/output/sqlite && sqlc generate && cd -
```

Expected: `internal/adapter/output/sqlite/db/` 아래 Go 파일 생성됨

- [ ] **Step 5: Repository 구현**

`internal/adapter/output/sqlite/repository.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"webhook-relay/internal/adapter/output/sqlite/db"
	"webhook-relay/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ interface {
	Save(context.Context, domain.Alert) error
} = (*Repository)(nil)

type Repository struct {
	queries *db.Queries
	sqlDB   *sql.DB
}

func New(dsn string) (*Repository, error) {
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := sqlDB.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Repository{queries: db.New(sqlDB), sqlDB: sqlDB}, nil
}

func (r *Repository) Close() error { return r.sqlDB.Close() }

func (r *Repository) Save(ctx context.Context, a domain.Alert) error {
	err := r.queries.InsertAlert(ctx, db.InsertAlertParams{
		ID:        a.ID,
		Version:   int64(a.Version),
		Source:    string(a.Source),
		Payload:   []byte(a.Payload),
		CreatedAt: a.CreatedAt.UTC(),
		Status:    string(a.Status),
		RetryCount: int64(a.RetryCount),
	})
	if err != nil {
		return fmt.Errorf("save alert: %w", err)
	}
	return nil
}

func (r *Repository) UpdateDeliveryState(ctx context.Context, id string, status domain.AlertStatus, retryCount int, lastAttemptAt time.Time) error {
	t := lastAttemptAt.UTC()
	err := r.queries.UpdateDeliveryState(ctx, db.UpdateDeliveryStateParams{
		Status:        string(status),
		RetryCount:    int64(retryCount),
		LastAttemptAt: sql.NullTime{Time: t, Valid: true},
		ID:            id,
	})
	if err != nil {
		return fmt.Errorf("update delivery state: %w", err)
	}
	return nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (domain.Alert, error) {
	row, err := r.queries.GetAlertByID(ctx, id)
	if err != nil {
		return domain.Alert{}, fmt.Errorf("find alert %q: %w", id, err)
	}
	return toAlert(row), nil
}

func (r *Repository) FindBySource(ctx context.Context, sourceID string, limit, offset int) ([]domain.Alert, error) {
	rows, err := r.queries.ListAlertsBySource(ctx, db.ListAlertsBySourceParams{
		Source: sourceID, Limit: int64(limit), Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	alerts := make([]domain.Alert, 0, len(rows))
	for _, row := range rows {
		alerts = append(alerts, toAlert(row))
	}
	return alerts, nil
}

func toAlert(row db.Alert) domain.Alert {
	a := domain.Alert{
		ID:         row.ID,
		Version:    int(row.Version),
		Source:     domain.SourceType(row.Source),
		Payload:    domain.RawPayload(row.Payload),
		CreatedAt:  row.CreatedAt,
		Status:     domain.AlertStatus(row.Status),
		RetryCount: int(row.RetryCount),
	}
	if row.LastAttemptAt.Valid {
		t := row.LastAttemptAt.Time
		a.LastAttemptAt = &t
	}
	return a
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS alerts (
    id              TEXT PRIMARY KEY,
    version         INTEGER NOT NULL DEFAULT 1,
    source          TEXT NOT NULL,
    payload         BLOB NOT NULL,
    created_at      DATETIME NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_attempt_at DATETIME
);`
```

- [ ] **Step 6: 테스트 통과 확인**

```bash
go test ./internal/adapter/output/sqlite/... -v
```

Expected: PASS (3 tests)

- [ ] **Step 7: 커밋**

```bash
git add internal/adapter/output/sqlite/
git commit -m "feat(adapter/sqlite): add alert repository with sqlc"
```

---

## Task 7: 파일 큐 어댑터 (AlertQueue)

**Files:**
- Create: `internal/adapter/output/filequeue/queue.go`
- Test: `internal/adapter/output/filequeue/queue_test.go`

> 주의: AckFunc/NackFunc는 `output` 패키지에 정의됨. filequeue는 output 패키지를 import한다.

- [ ] **Step 1: 테스트 작성**

`internal/adapter/output/filequeue/queue_test.go`:

```go
package filequeue_test

import (
	"context"
	"testing"

	"webhook-relay/internal/adapter/output/filequeue"
	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ output.AlertQueue = (*filequeue.Queue)(nil)

func TestQueue_EnqueueDequeueAck(t *testing.T) {
	q, _ := filequeue.New(t.TempDir())
	ctx := context.Background()

	alert := domain.Alert{ID: "q-001", Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{"x":1}`), Status: domain.AlertStatusPending, Version: 1}
	if err := q.Enqueue(ctx, alert); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, ack, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got.ID != alert.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, alert.ID)
	}
	if err := ack(); err != nil {
		t.Fatalf("ack: %v", err)
	}
}

func TestQueue_Nack_Requeues(t *testing.T) {
	q, _ := filequeue.New(t.TempDir())
	ctx := context.Background()

	alert := domain.Alert{ID: "q-002", Source: domain.SourceTypeDozzle, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending, Version: 1}
	q.Enqueue(ctx, alert)

	_, _, nack, _ := q.Dequeue(ctx)
	if err := nack(); err != nil {
		t.Fatalf("nack: %v", err)
	}

	got, ack, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("re-Dequeue: %v", err)
	}
	if got.ID != alert.ID {
		t.Errorf("re-dequeue ID: got %q, want %q", got.ID, alert.ID)
	}
	ack()
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/adapter/output/filequeue/...
```

Expected: FAIL

- [ ] **Step 3: 파일 큐 구현**

`internal/adapter/output/filequeue/queue.go`:

```go
package filequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ output.AlertQueue = (*Queue)(nil)

type Queue struct{ dir string }

func New(dir string) (*Queue, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir queue: %w", err)
	}
	return &Queue{dir: dir}, nil
}

func (q *Queue) Enqueue(_ context.Context, alert domain.Alert) error {
	b, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	name := fmt.Sprintf("%d-%s.json", time.Now().UnixNano(), alert.ID)
	if err := os.WriteFile(filepath.Join(q.dir, name), b, 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func (q *Queue) Dequeue(_ context.Context) (domain.Alert, output.AckFunc, output.NackFunc, error) {
	entries, _ := os.ReadDir(q.dir)
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return domain.Alert{}, nil, nil, fmt.Errorf("queue empty")
	}

	src := filepath.Join(q.dir, files[0])
	proc := src + ".processing"
	if err := os.Rename(src, proc); err != nil {
		return domain.Alert{}, nil, nil, fmt.Errorf("lock: %w", err)
	}
	b, err := os.ReadFile(proc)
	if err != nil {
		os.Rename(proc, src)
		return domain.Alert{}, nil, nil, fmt.Errorf("read: %w", err)
	}
	var alert domain.Alert
	if err := json.Unmarshal(b, &alert); err != nil {
		os.Rename(proc, src)
		return domain.Alert{}, nil, nil, fmt.Errorf("unmarshal: %w", err)
	}

	ack := output.AckFunc(func() error { return os.Remove(proc) })
	nack := output.NackFunc(func() error { return os.Rename(proc, src) })
	return alert, ack, nack, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

```bash
go test ./internal/adapter/output/filequeue/... -v
```

Expected: PASS (2 tests)

- [ ] **Step 5: 커밋**

```bash
git add internal/adapter/output/filequeue/
git commit -m "feat(adapter/filequeue): add at-least-once file queue"
```

---

## Task 8: AlertService (ReceiveAlertUseCase)

**Files:**
- Create: `internal/application/service/alert_service.go`
- Test: `internal/application/service/alert_service_test.go`

> ReceiveAlertUseCase.Receive는 `domain.SourceType`을 받는다 (string이 아님).

- [ ] **Step 1: 테스트 작성**

`internal/application/service/alert_service_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/application/service"
	"webhook-relay/internal/domain"
)

type mockRepo struct {
	saveFn func(context.Context, domain.Alert) error
}

func (m *mockRepo) Save(ctx context.Context, a domain.Alert) error { return m.saveFn(ctx, a) }
func (m *mockRepo) UpdateDeliveryState(_ context.Context, _ string, _ domain.AlertStatus, _ int, _ time.Time) error {
	return nil
}
func (m *mockRepo) FindByID(_ context.Context, _ string) (domain.Alert, error) {
	return domain.Alert{}, nil
}
func (m *mockRepo) FindBySource(_ context.Context, _ string, _, _ int) ([]domain.Alert, error) {
	return nil, nil
}

type mockQueue struct {
	enqueueFn func(context.Context, domain.Alert) error
}

func (m *mockQueue) Enqueue(ctx context.Context, a domain.Alert) error { return m.enqueueFn(ctx, a) }
func (m *mockQueue) Dequeue(_ context.Context) (domain.Alert, output.AckFunc, output.NackFunc, error) {
	return domain.Alert{}, nil, nil, nil
}

func TestAlertService_Receive_Success(t *testing.T) {
	var saved domain.Alert
	repo := &mockRepo{saveFn: func(_ context.Context, a domain.Alert) error { saved = a; return nil }}
	var enqueued domain.Alert
	queue := &mockQueue{enqueueFn: func(_ context.Context, a domain.Alert) error { enqueued = a; return nil }}

	svc := service.NewAlertService(repo, queue)
	id, err := svc.Receive(context.Background(), domain.SourceTypeBeszel, []byte(`{"host":"srv1"}`))
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}
	if id == "" {
		t.Error("returned ID should not be empty")
	}
	if saved.Source != domain.SourceTypeBeszel {
		t.Errorf("source = %q, want BESZEL", saved.Source)
	}
	if saved.Status != domain.AlertStatusPending {
		t.Errorf("status = %q, want PENDING", saved.Status)
	}
	if saved.ID != id {
		t.Errorf("saved.ID = %q, want returned ID %q", saved.ID, id)
	}
	if enqueued.ID != saved.ID {
		t.Errorf("enqueued ID != saved ID")
	}
}

func TestAlertService_Receive_SaveError(t *testing.T) {
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Alert) error { return errors.New("db err") }}
	queue := &mockQueue{enqueueFn: func(_ context.Context, _ domain.Alert) error { return nil }}

	svc := service.NewAlertService(repo, queue)
	if _, err := svc.Receive(context.Background(), domain.SourceTypeBeszel, []byte(`{}`)); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/application/service/... -run TestAlertService
```

Expected: FAIL

- [ ] **Step 3: AlertService 구현**

`internal/application/service/alert_service.go`:

```go
package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/domain"
)

type AlertService struct {
	repo  output.AlertRepository
	queue output.AlertQueue
}

func NewAlertService(repo output.AlertRepository, queue output.AlertQueue) *AlertService {
	return &AlertService{repo: repo, queue: queue}
}

func (s *AlertService) Receive(ctx context.Context, source domain.SourceType, payload []byte) (string, error) {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
	alert := domain.Alert{
		ID:        id,
		Version:   1,
		Source:    source,
		Payload:   domain.RawPayload(payload),
		CreatedAt: time.Now().UTC(),
		Status:    domain.AlertStatusPending,
	}
	if err := s.repo.Save(ctx, alert); err != nil {
		return "", fmt.Errorf("receive: save: %w", err)
	}
	if err := s.queue.Enqueue(ctx, alert); err != nil {
		return "", fmt.Errorf("receive: enqueue: %w", err)
	}
	return id, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

```bash
go test ./internal/application/service/... -run TestAlertService -v
```

Expected: PASS (2 tests)

- [ ] **Step 5: 커밋**

```bash
git add internal/application/service/alert_service.go internal/application/service/alert_service_test.go
git commit -m "feat(service): add alert receive use case"
```

---

## Task 9: Webhook Sender + SenderRegistry

**Files:**
- Create: `internal/adapter/output/webhook/sender.go`
- Create: `internal/adapter/output/webhook/registry.go`
- Test: `internal/adapter/output/webhook/sender_test.go`

> registry.Get() 반환 타입: `output.AlertSender` (named interface)

- [ ] **Step 1: 테스트 작성**

`internal/adapter/output/webhook/sender_test.go`:

```go
package webhook_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"webhook-relay/internal/adapter/output/webhook"
	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ output.AlertSender = (*webhook.Sender)(nil)
var _ output.SenderRegistry = (*webhook.Registry)(nil)

func TestSender_Send(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := webhook.NewSender()
	channel := domain.Channel{URL: srv.URL, Template: `{"text":"{{ .Source }}"}`}
	alert := domain.Alert{ID: "a1", Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{}`), Version: 1}

	if err := sender.Send(context.Background(), channel, alert); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if string(received) != `{"text":"BESZEL"}` {
		t.Errorf("body = %q", received)
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := webhook.NewRegistry(map[domain.ChannelType]output.AlertSender{
		domain.ChannelTypeWebhook: webhook.NewSender(),
	})
	got, err := reg.Get(domain.ChannelTypeWebhook)
	if err != nil || got == nil {
		t.Errorf("Get(WEBHOOK): err=%v", err)
	}
	_, err = reg.Get(domain.ChannelTypeSlack)
	if err == nil {
		t.Error("expected error for unregistered type")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/adapter/output/webhook/...
```

Expected: FAIL

- [ ] **Step 3: Sender 구현**

`internal/adapter/output/webhook/sender.go`:

```go
package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"webhook-relay/internal/domain"
)

type Sender struct{}

func NewSender() *Sender { return &Sender{} }

func (s *Sender) Send(ctx context.Context, ch domain.Channel, alert domain.Alert) error {
	body, err := domain.RenderTemplate(ch.Template, alert)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	client := &http.Client{}
	if ch.SkipTLSVerify {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ch.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if ch.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+ch.Secret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
```

`internal/adapter/output/webhook/registry.go`:

```go
package webhook

import (
	"fmt"

	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/domain"
)

type Registry struct {
	senders map[domain.ChannelType]output.AlertSender
}

func NewRegistry(senders map[domain.ChannelType]output.AlertSender) *Registry {
	return &Registry{senders: senders}
}

func (r *Registry) Get(t domain.ChannelType) (output.AlertSender, error) {
	s, ok := r.senders[t]
	if !ok {
		return nil, fmt.Errorf("get sender %q: %w", t, domain.ErrSenderNotFound)
	}
	return s, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

```bash
go test ./internal/adapter/output/webhook/... -v
```

Expected: PASS (2 tests)

- [ ] **Step 5: 커밋**

```bash
git add internal/adapter/output/webhook/
git commit -m "feat(adapter/webhook): add webhook sender and sender registry"
```

---

## Task 10: DeliveryWorker

**Files:**
- Create: `internal/application/service/delivery_worker.go`
- Test: `internal/application/service/delivery_worker_test.go`

- [ ] **Step 1: 테스트 작성**

`internal/application/service/delivery_worker_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/application/service"
	"webhook-relay/internal/domain"
)

type mockAlertQueue struct {
	alerts []domain.Alert
	idx    int
}

func (m *mockAlertQueue) Enqueue(_ context.Context, _ domain.Alert) error { return nil }
func (m *mockAlertQueue) Dequeue(_ context.Context) (domain.Alert, output.AckFunc, output.NackFunc, error) {
	if m.idx >= len(m.alerts) {
		time.Sleep(10 * time.Millisecond)
		return domain.Alert{}, nil, nil, errors.New("empty")
	}
	a := m.alerts[m.idx]
	m.idx++
	return a, func() error { return nil }, func() error { return nil }, nil
}

type mockRouteReader struct{ channels []domain.Channel }

func (m *mockRouteReader) GetChannels(_ context.Context, _ string) ([]domain.Channel, error) {
	return m.channels, nil
}

type mockSender struct{ count atomic.Int32 }

func (m *mockSender) Send(_ context.Context, _ domain.Channel, _ domain.Alert) error {
	m.count.Add(1)
	return nil
}

type mockRegistry struct{ sender *mockSender }

func (m *mockRegistry) Get(_ domain.ChannelType) (output.AlertSender, error) {
	return m.sender, nil
}

func TestDeliveryWorker_DeliverSuccess(t *testing.T) {
	alert := domain.Alert{ID: "w1", Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending, Version: 1}
	queue := &mockAlertQueue{alerts: []domain.Alert{alert}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Alert) error { return nil }}
	sender := &mockSender{}
	routeReader := &mockRouteReader{channels: []domain.Channel{{ID: "c1", Type: domain.ChannelTypeWebhook, Template: `{{ .Source }}`}}}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)
	worker.Start(ctx, 1)

	time.Sleep(150 * time.Millisecond)
	if sender.count.Load() == 0 {
		t.Error("expected at least one send call")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/application/service/... -run TestDeliveryWorker
```

Expected: FAIL

- [ ] **Step 3: DeliveryWorker 구현**

`internal/application/service/delivery_worker.go`:

```go
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/domain"
)

type DeliveryWorker struct {
	queue       output.AlertQueue
	repo        output.AlertRepository
	routeReader output.RouteConfigReader
	registry    output.SenderRegistry
}

func NewDeliveryWorker(
	queue output.AlertQueue,
	repo output.AlertRepository,
	routeReader output.RouteConfigReader,
	registry output.SenderRegistry,
) *DeliveryWorker {
	return &DeliveryWorker{queue: queue, repo: repo, routeReader: routeReader, registry: registry}
}

func (w *DeliveryWorker) Start(ctx context.Context, workerCount int) {
	for range workerCount {
		go w.loop(ctx)
	}
}

func (w *DeliveryWorker) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := w.processOne(ctx); err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(500 * time.Millisecond):
				}
			}
		}
	}
}

func (w *DeliveryWorker) processOne(ctx context.Context) error {
	alert, ack, nack, err := w.queue.Dequeue(ctx)
	if err != nil {
		return err
	}

	channels, err := w.routeReader.GetChannels(ctx, string(alert.Source))
	if err != nil {
		_ = nack()
		return fmt.Errorf("get channels: %w", err)
	}

	success := true
	for _, ch := range channels {
		if err := w.deliver(ctx, ch, alert); err != nil {
			slog.Warn("delivery failed", "alertID", alert.ID, "channel", ch.ID, "err", err)
			success = false
		}
	}

	now := time.Now().UTC()
	if success {
		if err := ack(); err != nil {
			slog.Warn("ack failed", "alertID", alert.ID, "err", err)
		}
		_ = w.repo.UpdateDeliveryState(ctx, alert.ID, domain.AlertStatusDelivered, alert.RetryCount, now)
	} else {
		_ = nack()
		_ = w.repo.UpdateDeliveryState(ctx, alert.ID, domain.AlertStatusFailed, alert.RetryCount+1, now)
	}
	return nil
}

func (w *DeliveryWorker) deliver(ctx context.Context, ch domain.Channel, alert domain.Alert) error {
	sender, err := w.registry.Get(ch.Type)
	if err != nil {
		return fmt.Errorf("get sender: %w", err)
	}
	retryCount, delayMs := ch.RetryCount, ch.RetryDelayMs
	if retryCount <= 0 {
		retryCount = 3
	}
	if delayMs <= 0 {
		delayMs = 1000
	}
	var lastErr error
	for i := range retryCount {
		if err := sender.Send(ctx, ch, alert); err == nil {
			return nil
		} else {
			lastErr = err
		}
		backoff := time.Duration(delayMs*(1<<uint(i))) * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return fmt.Errorf("retries exhausted: %w", lastErr)
}
```

- [ ] **Step 4: 테스트 통과 확인 (race detector 포함)**

```bash
go test -race ./internal/application/service/... -v
```

Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/application/service/delivery_worker.go internal/application/service/delivery_worker_test.go
git commit -m "feat(service): add delivery worker with exponential backoff"
```

---

## Task 11: HTTP 입력 어댑터

**Files:**
- Create: `internal/adapter/input/http/middleware.go`
- Create: `internal/adapter/input/http/source_resolver.go` ← URL sourceID → SourceType 변환
- Create: `internal/adapter/input/http/handler.go`
- Create: `internal/adapter/input/http/router.go` ← X-API-Version 미들웨어
- Test: `internal/adapter/input/http/handler_test.go`

- [ ] **Step 1: 테스트 작성**

`internal/adapter/input/http/handler_test.go`:

```go
package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "webhook-relay/internal/adapter/input/http"
	"webhook-relay/internal/domain"
)

type mockUseCase struct {
	receiveFn func(context.Context, domain.SourceType, []byte) (string, error)
}

func (m *mockUseCase) Receive(ctx context.Context, s domain.SourceType, p []byte) (string, error) {
	return m.receiveFn(ctx, s, p)
}

type mockResolver struct {
	sources map[string]domain.SourceType
	secrets map[string]string
}

func (m *mockResolver) Resolve(sourceID string) (domain.SourceType, error) {
	st, ok := m.sources[sourceID]
	if !ok {
		return "", domain.ErrSourceNotFound
	}
	return st, nil
}

func (m *mockResolver) ValidateToken(sourceID, token string) bool {
	return m.secrets[sourceID] == token
}

func newTestRouter(receiveFn func(context.Context, domain.SourceType, []byte) (string, error)) http.Handler {
	uc := &mockUseCase{receiveFn: receiveFn}
	resolver := &mockResolver{
		sources: map[string]domain.SourceType{"beszel": domain.SourceTypeBeszel},
		secrets: map[string]string{"beszel": "test-token"},
	}
	return httpadapter.NewRouter(uc, resolver, nil)
}

func TestHandler_PostAlert_Success(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
		return "01JTEST00000000000000000", nil
	})
	req := httptest.NewRequest(http.MethodPost, "/sources/beszel/alerts", strings.NewReader(`{"level":"critical"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc == "" {
		t.Error("Location header missing")
	}
	// Location must point to the specific alert, not the collection
	if !strings.Contains(loc, "/alerts/01JTEST00000000000000000") {
		t.Errorf("Location = %q, want path containing specific alertId", loc)
	}
	if v := w.Header().Get("X-API-Version"); v == "" {
		t.Error("X-API-Version header missing")
	}
}

func TestHandler_PostAlert_InvalidToken(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
		return "", nil
	})
	req := httptest.NewRequest(http.MethodPost, "/sources/beszel/alerts", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer wrong-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestHandler_Healthz(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
		return "", nil
	})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandler_SourceNotFound(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
		return "", domain.ErrSourceNotFound
	})
	req := httptest.NewRequest(http.MethodPost, "/sources/unknown/alerts", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		// unknown source → token check fails first → 401
		t.Logf("status = %d (expected 401 since unknown source has no registered token)", w.Code)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/adapter/input/http/...
```

Expected: FAIL

- [ ] **Step 3: SourceResolver 구현**

`internal/adapter/input/http/source_resolver.go`:

```go
package http

import "webhook-relay/internal/domain"

// SourceResolver URL sourceID를 domain.SourceType으로 변환하고 토큰을 검증한다.
type SourceResolver interface {
	Resolve(sourceID string) (domain.SourceType, error)
	ValidateToken(sourceID, token string) bool
}
```

- [ ] **Step 4: 미들웨어 구현**

`internal/adapter/input/http/middleware.go`:

```go
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"webhook-relay/internal/domain"
)

type contextKey string

const traceIDKey contextKey = "traceID"

type errorResponse struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Status  int    `json:"status"`
	Detail  string `json:"detail"`
	TraceID string `json:"traceId,omitempty"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	traceID, _ := r.Context().Value(traceIDKey).(string)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{
		Type: "/errors/" + http.StatusText(status), Title: title,
		Status: status, Detail: detail, TraceID: traceID,
	})
}

func mapError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidToken):
		writeError(w, r, http.StatusUnauthorized, "Unauthorized", err.Error())
	case errors.Is(err, domain.ErrSourceNotFound):
		writeError(w, r, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, domain.ErrAlertNotFound):
		writeError(w, r, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, r, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error())
	default:
		writeError(w, r, http.StatusInternalServerError, "Internal Server Error", "unexpected error")
	}
}

func apiVersionMiddleware(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-API-Version", version)
			next.ServeHTTP(w, r)
		})
	}
}

func tokenFromHeader(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}

func withTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey, id)
}
```

- [ ] **Step 5: 핸들러 구현**

`internal/adapter/input/http/handler.go`:

```go
package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"webhook-relay/internal/application/port/input"
	"webhook-relay/internal/domain"
)

type Handler struct {
	uc       input.ReceiveAlertUseCase
	resolver SourceResolver
}

func NewHandler(uc input.ReceiveAlertUseCase, resolver SourceResolver) *Handler {
	return &Handler{uc: uc, resolver: resolver}
}

func (h *Handler) PostAlert(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceId")
	token := tokenFromHeader(r)

	if !h.resolver.ValidateToken(sourceID, token) {
		writeError(w, r, http.StatusUnauthorized, "Unauthorized",
			fmt.Sprintf("invalid or missing token for source: %s", sourceID))
		return
	}

	sourceType, err := h.resolver.Resolve(sourceID)
	if err != nil {
		mapError(w, r, err)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Bad Request", "failed to read body")
		return
	}

	alertID, err := h.uc.Receive(r.Context(), sourceType, body)
	if err != nil {
		mapError(w, r, err)
		return
	}

	resp := map[string]any{
		"id":        alertID,
		"sourceId":  sourceID,
		"status":    string(domain.AlertStatusPending),
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", fmt.Sprintf("/sources/%s/alerts/%s", sourceID, alertID))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 6: 라우터 구현 (리터럴 경로 우선 등록)**

`internal/adapter/input/http/router.go`:

```go
package http

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"webhook-relay/internal/adapter/input/websocket"
	"webhook-relay/internal/application/port/input"
	"webhook-relay/internal/domain"
)

// API 버전 — X-API-Version 헤더로 반환
const APIVersion = "2026-03-20"

// WSHandler is the subset of websocket.Handler used by the router.
// nil is allowed for tests that don't exercise the /alerts/ws path.
type WSHandler interface {
	ServeWS(w http.ResponseWriter, r *http.Request, source domain.SourceType)
}

func NewRouter(uc input.ReceiveAlertUseCase, resolver SourceResolver, ws WSHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(apiVersionMiddleware(APIVersion))

	h := NewHandler(uc, resolver)

	r.Get("/healthz", h.Healthz)

	r.Route("/sources/{sourceId}", func(r chi.Router) {
		// 리터럴 /alerts/ws를 와일드카드 /alerts/{alertId}보다 먼저 등록
		r.Get("/alerts/ws", func(w http.ResponseWriter, req *http.Request) {
			sourceID := chi.URLParam(req, "sourceId")
			sourceType, err := resolver.Resolve(sourceID)
			if err != nil {
				writeError(w, req, http.StatusUnauthorized, "Unauthorized", "unknown source")
				return
			}
			if ws == nil {
				writeError(w, req, http.StatusNotImplemented, "Not Implemented", "websocket not configured")
				return
			}
			ws.ServeWS(w, req, sourceType)
		})
		r.Post("/alerts", h.PostAlert)
		r.Get("/alerts/{alertId}", h.Healthz) // placeholder
	})

	return r
}

// Ensure *websocket.Handler satisfies WSHandler at compile time.
var _ WSHandler = (*websocket.Handler)(nil)
```

- [ ] **Step 7: 테스트 통과 확인**

```bash
go test ./internal/adapter/input/http/... -v
```

Expected: PASS (4 tests)

- [ ] **Step 8: 커밋**

```bash
git add internal/adapter/input/http/
git commit -m "feat(adapter/http): add chi router with RFC 7807 errors and X-API-Version"
```

---

## Task 12: WebSocket 입력 어댑터

**Files:**
- Create: `internal/adapter/input/websocket/handler.go`
- Test: `internal/adapter/input/websocket/handler_test.go`

- [ ] **Step 1: 테스트 작성**

`internal/adapter/input/websocket/handler_test.go`:

```go
package websocket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gws "github.com/gorilla/websocket"
	wsadapter "webhook-relay/internal/adapter/input/websocket"
	"webhook-relay/internal/domain"
)

type mockUseCase struct{ count atomic.Int32 }

func (m *mockUseCase) Receive(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
	m.count.Add(1)
	return "test-id", nil
}

func TestWebSocketHandler_ReceiveMessage(t *testing.T) {
	uc := &mockUseCase{}
	handler := wsadapter.NewHandler(uc)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeWS(w, r, domain.SourceTypeBeszel)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := gws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(gws.TextMessage, []byte(`{"host":"srv1"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn.Close()
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/adapter/input/websocket/...
```

Expected: FAIL

- [ ] **Step 3: WebSocket 핸들러 구현**

`internal/adapter/input/websocket/handler.go`:

```go
package websocket

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"webhook-relay/internal/application/port/input"
	"webhook-relay/internal/domain"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

type Handler struct{ uc input.ReceiveAlertUseCase }

func NewHandler(uc input.ReceiveAlertUseCase) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request, source domain.SourceType) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("ws read error", "source", source, "err", err)
			}
			return
		}
		if _, err := h.uc.Receive(r.Context(), source, msg); err != nil {
			slog.Warn("receive via ws failed", "source", source, "err", err)
		}
	}
}
```

- [ ] **Step 4: 테스트 통과 확인**

```bash
go test ./internal/adapter/input/websocket/... -v
```

Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/adapter/input/websocket/
git commit -m "feat(adapter/websocket): add inbound websocket handler"
```

---

## Task 13: DI 조립 (cmd/server/main.go)

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: main.go 구현**

`cmd/server/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	cfgpkg "webhook-relay/internal/config"
	sqliteadapter "webhook-relay/internal/adapter/output/sqlite"
	"webhook-relay/internal/adapter/output/filequeue"
	webhookadapter "webhook-relay/internal/adapter/output/webhook"
	httpadapter "webhook-relay/internal/adapter/input/http"
	wsadapter "webhook-relay/internal/adapter/input/websocket"
	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/application/service"
	"webhook-relay/internal/domain"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var cfgPath string
	root := &cobra.Command{Use: "webhook-relay", Short: "Monitoring alert relay hub"}
	start := &cobra.Command{
		Use:   "start",
		Short: "Start server",
		RunE:  func(_ *cobra.Command, _ []string) error { return runServer(cfgPath) },
	}
	start.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "config file path")
	root.AddCommand(start)
	root.AddCommand(&cobra.Command{
		Use: "version", Short: "Print version",
		Run: func(_ *cobra.Command, _ []string) { fmt.Println("webhook-relay v0.1.0") },
	})
	return root
}

func runServer(cfgPath string) error {
	cfg, err := cfgpkg.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	setupLogger(cfg)

	// 아웃바운드 어댑터
	repo, err := sqliteadapter.New(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("init sqlite: %w", err)
	}
	defer repo.Close()

	queue, err := filequeue.New(cfg.Queue.Path)
	if err != nil {
		return fmt.Errorf("init queue: %w", err)
	}

	sender := webhookadapter.NewSender()
	registry := webhookadapter.NewRegistry(map[domain.ChannelType]output.AlertSender{
		domain.ChannelTypeWebhook: sender,
	})

	// 설정 기반 라우팅 (핫리로드 지원)
	routeReader := cfgpkg.NewInMemoryRouteConfigReader(cfg)

	// Viper WatchConfig → 핫리로드
	v, err := cfgpkg.NewViper(cfgPath)
	if err != nil {
		return fmt.Errorf("init viper: %w", err)
	}
	cfgpkg.Watch(v, func(newCfg *cfgpkg.Config) {
		routeReader.Update(newCfg)
		slog.Info("config reloaded")
	})

	// 애플리케이션 서비스
	alertSvc := service.NewAlertService(repo, queue)
	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)

	// HTTP + WebSocket 어댑터 조립
	resolver := newConfigSourceResolver(cfg)
	wsHandler := wsadapter.NewHandler(alertSvc)
	router := httpadapter.NewRouter(alertSvc, resolver, wsHandler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker.Start(ctx, cfg.Queue.WorkerCount)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
		}
	}()

	<-sig
	slog.Info("shutting down")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	return srv.Shutdown(shutCtx)
}

// configSourceResolver Config 기반 SourceResolver 구현
type configSourceResolver struct {
	sources map[string]domain.SourceType
	secrets map[string]string
}

func newConfigSourceResolver(cfg *cfgpkg.Config) *configSourceResolver {
	sources := make(map[string]domain.SourceType, len(cfg.Sources))
	secrets := make(map[string]string, len(cfg.Sources))
	for _, s := range cfg.Sources {
		sources[s.ID] = domain.SourceType(s.Type)
		secrets[s.ID] = s.Secret
	}
	return &configSourceResolver{sources: sources, secrets: secrets}
}

func (r *configSourceResolver) Resolve(sourceID string) (domain.SourceType, error) {
	st, ok := r.sources[sourceID]
	if !ok {
		return "", fmt.Errorf("resolve %q: %w", sourceID, domain.ErrSourceNotFound)
	}
	return st, nil
}

func (r *configSourceResolver) ValidateToken(sourceID, token string) bool {
	return r.secrets[sourceID] == token
}

func setupLogger(cfg *cfgpkg.Config) {
	level := map[string]slog.Level{
		"debug": slog.LevelDebug, "warn": slog.LevelWarn, "error": slog.LevelError,
	}[cfg.Log.Level]
	var h slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	if cfg.Log.Format == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(h))
}
```

- [ ] **Step 2: 빌드 확인**

```bash
go build ./cmd/server/
```

Expected: 에러 없음

- [ ] **Step 3: version 커맨드 확인**

```bash
./webhook-relay version
```

Expected: `webhook-relay v0.1.0`

- [ ] **Step 4: 커밋**

```bash
git add cmd/server/main.go
git commit -m "feat(cmd): add cobra CLI with full DI assembly and hot-reload"
```

---

## Task 14: E2E 테스트

**Files:**
- Create: `test/e2e/e2e_test.go`

- [ ] **Step 1: E2E 테스트 작성**

`test/e2e/e2e_test.go`:

```go
package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cfgpkg "webhook-relay/internal/config"
	sqliteadapter "webhook-relay/internal/adapter/output/sqlite"
	"webhook-relay/internal/adapter/output/filequeue"
	webhookadapter "webhook-relay/internal/adapter/output/webhook"
	httpadapter "webhook-relay/internal/adapter/input/http"
	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/application/service"
	"webhook-relay/internal/domain"
)

// configSourceResolver는 cmd/server/main.go와 동일한 로직을 E2E에서 재구현 (DI 검증용)
type configSourceResolver struct {
	sources map[string]domain.SourceType
	secrets map[string]string
}

func (r *configSourceResolver) Resolve(id string) (domain.SourceType, error) {
	st, ok := r.sources[id]
	if !ok {
		return "", domain.ErrSourceNotFound
	}
	return st, nil
}

func (r *configSourceResolver) ValidateToken(id, token string) bool {
	return r.secrets[id] == token
}

func TestE2E_PostAlert_Returns201(t *testing.T) {
	// 아웃바운드 웹훅 수신 서버
	var deliveredPayload []byte
	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveredPayload, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer targetSrv.Close()

	cfg := &cfgpkg.Config{
		Sources:  []cfgpkg.SourceConfig{{ID: "beszel", Type: "BESZEL", Secret: "tok"}},
		Channels: []cfgpkg.ChannelConfig{{ID: "ch1", Type: "WEBHOOK", URL: targetSrv.URL, Template: `{"src":"{{ .Source }}"}`, RetryCount: 1, RetryDelayMs: 10}},
		Routes:   []cfgpkg.RouteConfig{{SourceID: "beszel", ChannelIDs: []string{"ch1"}}},
		Queue:    cfgpkg.QueueConfig{WorkerCount: 1},
	}

	repo, _ := sqliteadapter.New(":memory:")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	sender := webhookadapter.NewSender()
	registry := webhookadapter.NewRegistry(map[domain.ChannelType]output.AlertSender{
		domain.ChannelTypeWebhook: sender,
	})
	routeReader := cfgpkg.NewInMemoryRouteConfigReader(cfg)
	alertSvc := service.NewAlertService(repo, queue)
	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)

	resolver := &configSourceResolver{
		sources: map[string]domain.SourceType{"beszel": domain.SourceTypeBeszel},
		secrets: map[string]string{"beszel": "tok"},
	}
	router := httpadapter.NewRouter(alertSvc, resolver, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker.Start(ctx, 1)

	// POST 알람
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/sources/beszel/alerts",
		strings.NewReader(`{"host":"server1"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if v := resp.Header.Get("X-API-Version"); v == "" {
		t.Error("X-API-Version header missing")
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "PENDING" {
		t.Errorf("body status = %v, want PENDING", body["status"])
	}

	// DeliveryWorker가 전달 완료할 때까지 대기
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(deliveredPayload) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(deliveredPayload) == 0 {
		t.Error("delivery worker did not deliver the alert")
	}
	want := fmt.Sprintf(`{"src":"%s"}`, string(domain.SourceTypeBeszel))
	if string(deliveredPayload) != want {
		t.Errorf("delivered payload = %q, want %q", deliveredPayload, want)
	}
}

func TestE2E_Healthz(t *testing.T) {
	cfg := &cfgpkg.Config{}
	repo, _ := sqliteadapter.New(":memory:")
	defer repo.Close()
	queue, _ := filequeue.New(t.TempDir())
	alertSvc := service.NewAlertService(repo, queue)
	resolver := &configSourceResolver{}
	router := httpadapter.NewRouter(alertSvc, resolver, nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", resp.StatusCode)
	}
}
```

- [ ] **Step 2: E2E 테스트 실행**

```bash
go test ./test/e2e/... -v -timeout 30s
```

Expected: PASS

- [ ] **Step 3: 전체 테스트 + race detector**

```bash
go test -race ./... -timeout 60s
```

Expected: 전체 PASS, race condition 없음

- [ ] **Step 4: vet 검사**

```bash
go vet ./...
```

Expected: 에러 없음

- [ ] **Step 5: 최종 빌드**

```bash
go build -o webhook-relay ./cmd/server/
./webhook-relay version
```

Expected: `webhook-relay v0.1.0`

- [ ] **Step 6: 최종 커밋**

```bash
git add test/e2e/
git commit -m "test(e2e): add end-to-end test covering full alert flow"
```

---

## 완료 기준

- [ ] `go vet ./...` 에러 없음
- [ ] `go test -race ./...` 전체 PASS
- [ ] `go build -o webhook-relay ./cmd/server/` 성공
- [ ] `./webhook-relay version` → `webhook-relay v0.1.0`
- [ ] E2E: `POST /sources/beszel/alerts` → 201 반환 + DeliveryWorker 아웃바운드 전달 완료
- [ ] E2E: `GET /healthz` → 200 반환

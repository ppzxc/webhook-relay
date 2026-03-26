package filequeue_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"relaybox/internal/adapter/output/filequeue"
	"relaybox/internal/application/port/output"
	"relaybox/internal/domain"
	"relaybox/internal/testutil"
)

// 컴파일 타임 인터페이스 검증
var _ output.MessageQueue = (*filequeue.Queue)(nil)

func TestQueue_EnqueueDequeueAck(t *testing.T) {
	q, _ := filequeue.New(t.TempDir())
	ctx := context.Background()

	msg := domain.Message{ID: "q-001", Input: "beszel", Payload: domain.RawPayload(`{"x":1}`), Status: domain.MessageStatusPending, Version: 1}
	if err := q.Enqueue(ctx, msg); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, ack, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got.ID != msg.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, msg.ID)
	}
	if err := ack(); err != nil {
		t.Fatalf("ack: %v", err)
	}
}

func TestQueue_New_RecoverOrphans(t *testing.T) {
	dir := t.TempDir()

	// 크래시를 시뮬레이션: .json.processing 고아 파일을 직접 생성
	msg := domain.Message{ID: "orphan-001", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	tmp, _ := filequeue.New(dir)
	ctx := context.Background()
	tmp.Enqueue(ctx, msg)
	// Dequeue로 .processing 상태로 전환 후 ack/nack 없이 종료 (크래시 시뮬)
	_, _, _, _ = tmp.Dequeue(ctx)

	// 고아 .processing 파일이 존재하는지 확인
	entries, _ := os.ReadDir(dir)
	var processingCount int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json.processing") {
			processingCount++
		}
	}
	if processingCount == 0 {
		t.Fatal("test setup failed: no orphan .processing file created")
	}

	// New() 재호출 시 복구되어야 한다
	q, err := filequeue.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// 복구 후 Dequeue 가능해야 함
	got, ack, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue after recovery: %v", err)
	}
	if got.ID != msg.ID {
		t.Errorf("ID = %q, want %q", got.ID, msg.ID)
	}
	ack()
}

func TestQueue_Enqueue_LogsDebug(t *testing.T) {
	captureH := &testutil.CaptureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(captureH))
	defer slog.SetDefault(orig)

	q, _ := filequeue.New(t.TempDir())
	msg := domain.Message{ID: "log-001", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	if err := q.Enqueue(context.Background(), msg); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	records := captureH.Records()
	found := false
	for _, r := range records {
		if r.Level == slog.LevelDebug && r.Msg == "message enqueued" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected DEBUG log 'message enqueued', got records: %v", records)
	}
}

func TestQueue_Dequeue_LogsDebug(t *testing.T) {
	q, _ := filequeue.New(t.TempDir())
	msg := domain.Message{ID: "log-002", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	q.Enqueue(context.Background(), msg) //nolint

	captureH := &testutil.CaptureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(captureH))
	defer slog.SetDefault(orig)

	if _, ack, _, err := q.Dequeue(context.Background()); err != nil {
		t.Fatalf("Dequeue: %v", err)
	} else {
		ack() //nolint
	}

	records := captureH.Records()
	found := false
	for _, r := range records {
		if r.Level == slog.LevelDebug && r.Msg == "message dequeued" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected DEBUG log 'message dequeued', got records: %v", records)
	}
}

func TestQueue_Dequeue_ReadDirError(t *testing.T) {
	dir := t.TempDir()
	q, err := filequeue.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// 큐 디렉토리를 제거해 ReadDir 에러를 유발한다.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	_, _, _, err = q.Dequeue(context.Background())
	if err == nil {
		t.Fatal("expected error from Dequeue when queue dir is removed, got nil")
	}
	if errors.Is(err, output.ErrQueueEmpty) {
		t.Errorf("expected readdir error, got ErrQueueEmpty")
	}
}

func TestQueue_RecoverOrphans_ReadDirError(t *testing.T) {
	captureH := &testutil.CaptureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(captureH))
	defer slog.SetDefault(orig)

	dir := t.TempDir()

	// recoverOrphans가 호출되는 New()를 dir 제거 후 실행하면 ReadDir 에러가 발생한다.
	// 단, New()는 MkdirAll을 먼저 호출하므로, 권한을 제거해 ReadDir 에러를 유발한다.
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(dir, 0755) //nolint

	// New()는 MkdirAll이 먼저 성공하지만 recoverOrphans의 ReadDir는 실패해야 한다.
	// 권한 0000이면 ReadDir도 실패한다.
	filequeue.New(dir) //nolint — 에러 여부와 무관하게 로그만 확인

	records := captureH.Records()
	found := false
	for _, r := range records {
		if r.Level == slog.LevelWarn && r.Msg == "failed to read queue directory" {
			found = true
			break
		}
	}
	if !found {
		t.Logf("records: %v (may pass on root or some systems)", records)
		// root 계정이나 특정 OS에서는 chmod가 효과 없을 수 있으므로 skip 처리
		if os.Getuid() == 0 {
			t.Skip("running as root, chmod test not effective")
		}
		t.Errorf("expected WARN log 'failed to read queue directory'")
	}
}

func TestQueue_Nack_Requeues(t *testing.T) {
	q, _ := filequeue.New(t.TempDir())
	ctx := context.Background()

	msg := domain.Message{ID: "q-002", Input: "dozzle", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	q.Enqueue(ctx, msg)

	_, _, nack, _ := q.Dequeue(ctx)
	if err := nack(); err != nil {
		t.Fatalf("nack: %v", err)
	}

	got, ack, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("re-Dequeue: %v", err)
	}
	if got.ID != msg.ID {
		t.Errorf("re-dequeue ID: got %q, want %q", got.ID, msg.ID)
	}
	ack()
}

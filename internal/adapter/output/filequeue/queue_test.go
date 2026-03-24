package filequeue_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"relaybox/internal/adapter/output/filequeue"
	"relaybox/internal/application/port/output"
	"relaybox/internal/domain"
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

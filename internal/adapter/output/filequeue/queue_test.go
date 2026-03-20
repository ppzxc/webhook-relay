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

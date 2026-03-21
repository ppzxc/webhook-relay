package tcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"relaybox/internal/domain"
)

// mockReceiveUseCase records calls to Receive.
type mockReceiveUseCase struct {
	mu       sync.Mutex
	calls    []receiveCall
	returnID string
	returnErr error
}

type receiveCall struct {
	inputType   domain.InputType
	contentType string
	body        []byte
}

func (m *mockReceiveUseCase) Receive(_ context.Context, inputType domain.InputType, contentType string, body []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(body))
	copy(cp, body)
	m.calls = append(m.calls, receiveCall{inputType: inputType, contentType: contentType, body: cp})
	return m.returnID, m.returnErr
}

func (m *mockReceiveUseCase) getCalls() []receiveCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]receiveCall, len(m.calls))
	copy(out, m.calls)
	return out
}

func startTestListener(t *testing.T, delimiter byte, contentType string, mock *mockReceiveUseCase) (addr string, cancel context.CancelFunc) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr = ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	listener := NewListener(mock, domain.InputTypeGeneric, ":0", delimiter, contentType)

	errCh := make(chan error, 1)
	go func() {
		errCh <- listener.StartWithListener(ctx, ln)
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("listener returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("listener did not stop in time")
		}
	})

	return addr, cancel
}

func waitForCalls(mock *mockReceiveUseCase, expected int, timeout time.Duration) ([]receiveCall, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		calls := mock.getCalls()
		if len(calls) >= expected {
			return calls, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	calls := mock.getCalls()
	return calls, fmt.Errorf("expected %d calls, got %d", expected, len(calls))
}

func TestListener_BasicMessageDelivery(t *testing.T) {
	mock := &mockReceiveUseCase{returnID: "msg-1"}
	addr, _ := startTestListener(t, '\n', "application/json", mock)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	calls, err := waitForCalls(mock, 1, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls[0].body) != "hello" {
		t.Errorf("body = %q, want %q", calls[0].body, "hello")
	}
	if calls[0].inputType != domain.InputTypeGeneric {
		t.Errorf("inputType = %q, want %q", calls[0].inputType, domain.InputTypeGeneric)
	}
	if calls[0].contentType != "application/json" {
		t.Errorf("contentType = %q, want %q", calls[0].contentType, "application/json")
	}
}

func TestListener_MultipleMessages(t *testing.T) {
	mock := &mockReceiveUseCase{returnID: "msg-1"}
	addr, _ := startTestListener(t, '\n', "application/json", mock)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("msg1\nmsg2\nmsg3\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	calls, err := waitForCalls(mock, 3, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"msg1", "msg2", "msg3"}
	for i, want := range expected {
		if string(calls[i].body) != want {
			t.Errorf("call[%d] body = %q, want %q", i, calls[i].body, want)
		}
	}
}

func TestListener_CustomDelimiter(t *testing.T) {
	mock := &mockReceiveUseCase{returnID: "msg-1"}
	addr, _ := startTestListener(t, '|', "application/json", mock)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("msg1|msg2|"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	calls, err := waitForCalls(mock, 2, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls[0].body) != "msg1" {
		t.Errorf("call[0] body = %q, want %q", calls[0].body, "msg1")
	}
	if string(calls[1].body) != "msg2" {
		t.Errorf("call[1] body = %q, want %q", calls[1].body, "msg2")
	}
}

func TestListener_GracefulShutdown(t *testing.T) {
	mock := &mockReceiveUseCase{returnID: "msg-1"}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	listener := NewListener(mock, domain.InputTypeGeneric, ":0", '\n', "application/json")

	errCh := make(chan error, 1)
	go func() {
		errCh <- listener.StartWithListener(ctx, ln)
	}()

	// Connect a client to verify listener is running
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()

	// Cancel context for graceful shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("expected nil error on graceful shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not stop in time")
	}
}

func TestListener_MaxMessageSize_Accepted(t *testing.T) {
	// A message body of exactly (maxMessageBytes - 1) bytes (+ delimiter) must be delivered.
	mock := &mockReceiveUseCase{returnID: "msg-1"}
	addr, _ := startTestListener(t, '\n', "application/json", mock)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// payload = maxMessageBytes - 1 bytes, then delimiter
	payload := make([]byte, maxMessageBytes-1)
	for i := range payload {
		payload[i] = 'x'
	}
	payload = append(payload, '\n')

	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	calls, err := waitForCalls(mock, 1, 3*time.Second)
	if err != nil {
		t.Fatalf("message not received: %v", err)
	}
	if len(calls[0].body) != maxMessageBytes-1 {
		t.Errorf("body len = %d, want %d", len(calls[0].body), maxMessageBytes-1)
	}
}

func TestListener_MaxMessageSize_Exceeded(t *testing.T) {
	// A message body exceeding maxMessageBytes must be silently dropped by the scanner.
	mock := &mockReceiveUseCase{returnID: "msg-1"}
	addr, _ := startTestListener(t, '\n', "application/json", mock)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// payload = maxMessageBytes + 1 bytes, then delimiter — scanner will reject this
	payload := make([]byte, maxMessageBytes+1)
	for i := range payload {
		payload[i] = 'x'
	}
	payload = append(payload, '\n')

	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait and verify no calls were recorded
	time.Sleep(500 * time.Millisecond)
	calls := mock.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for oversized message, got %d", len(calls))
	}
}

func TestListener_EmptyMessageSkip(t *testing.T) {
	mock := &mockReceiveUseCase{returnID: "msg-1"}
	addr, _ := startTestListener(t, '\n', "application/json", mock)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Send empty lines followed by a real message
	_, err = conn.Write([]byte("\n\nactual\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	calls, err := waitForCalls(mock, 1, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls[0].body) != "actual" {
		t.Errorf("body = %q, want %q", calls[0].body, "actual")
	}

	// Wait a bit more and verify no extra calls from empty messages
	time.Sleep(200 * time.Millisecond)
	finalCalls := mock.getCalls()
	if len(finalCalls) != 1 {
		t.Errorf("expected exactly 1 call, got %d", len(finalCalls))
	}
	conn.Close()
}

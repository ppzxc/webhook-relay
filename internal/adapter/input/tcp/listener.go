package tcp

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"

	"relaybox/internal/application/port/input"
	"relaybox/internal/domain"
)

// Listener accepts TCP connections and delivers delimiter-framed messages
// to the ReceiveMessageUseCase.
type Listener struct {
	uc          input.ReceiveMessageUseCase
	inputType   domain.InputType
	addr        string
	delimiter   byte
	contentType string
}

// NewListener creates a new TCP listener.
// contentType maps from the parser config (e.g. "application/json" for json parser).
func NewListener(
	uc input.ReceiveMessageUseCase,
	inputType domain.InputType,
	addr string,
	delimiter byte,
	contentType string,
) *Listener {
	return &Listener{
		uc:          uc,
		inputType:   inputType,
		addr:        addr,
		delimiter:   delimiter,
		contentType: contentType,
	}
}

// Start binds to the configured TCP address and accepts connections.
// Blocks until ctx is cancelled.
func (l *Listener) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", l.addr)
	if err != nil {
		return fmt.Errorf("tcp listen %s: %w", l.addr, err)
	}
	return l.StartWithListener(ctx, ln)
}

// StartWithListener accepts connections on the provided net.Listener.
// This is useful for testing with a pre-bound listener.
// Blocks until ctx is cancelled.
func (l *Listener) StartWithListener(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil // graceful shutdown
			default:
				slog.Error("tcp accept error", "addr", l.addr, "err", err)
				return fmt.Errorf("tcp accept: %w", err)
			}
		}
		go l.handleConn(ctx, conn)
	}
}

const maxMessageBytes = 1 << 20 // 1 MiB

func (l *Listener) handleConn(ctx context.Context, conn net.Conn) {
	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel() // exits the watcher goroutine when connection ends
	defer conn.Close()

	go func() {
		<-connCtx.Done()
		conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Split(splitFunc(l.delimiter))
	scanner.Buffer(make([]byte, 4096), maxMessageBytes)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Copy the bytes since scanner reuses the buffer
		msg := make([]byte, len(line))
		copy(msg, line)
		if _, err := l.uc.Receive(ctx, l.inputType, l.contentType, msg); err != nil {
			slog.Warn("tcp receive error", "inputType", l.inputType, "err", err)
		}
	}
	if err := scanner.Err(); err != nil {
		select {
		case <-ctx.Done():
			// expected on shutdown
		default:
			slog.Warn("tcp scanner error", "inputType", l.inputType, "err", err)
		}
	}
}

// splitFunc returns a bufio.SplitFunc that splits on the given delimiter byte.
func splitFunc(delim byte) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		for i, b := range data {
			if b == delim {
				return i + 1, data[:i], nil
			}
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
}

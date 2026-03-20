package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"relaybox/internal/application/port/output"
	"relaybox/internal/domain"
)

type RelayWorker struct {
	queue      output.MessageQueue
	repo       output.MessageRepository
	ruleReader output.RuleConfigReader
	registry   output.OutputRegistry
	wg         sync.WaitGroup
}

func NewRelayWorker(
	queue output.MessageQueue,
	repo output.MessageRepository,
	ruleReader output.RuleConfigReader,
	registry output.OutputRegistry,
) *RelayWorker {
	return &RelayWorker{queue: queue, repo: repo, ruleReader: ruleReader, registry: registry}
}

func (w *RelayWorker) Start(ctx context.Context, workerCount int) {
	w.wg.Add(workerCount)
	for range workerCount {
		go w.loop(ctx)
	}
}

// Wait blocks until all workers finish. Call after cancelling the context.
func (w *RelayWorker) Wait() {
	w.wg.Wait()
}

func (w *RelayWorker) loop(ctx context.Context) {
	defer w.wg.Done()
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

func (w *RelayWorker) processOne(ctx context.Context) error {
	msg, ack, nack, err := w.queue.Dequeue(ctx)
	if err != nil {
		return err
	}

	outputs, err := w.ruleReader.GetOutputs(ctx, string(msg.Input))
	if err != nil {
		_ = nack()
		return fmt.Errorf("get outputs: %w", err)
	}

	success := true
	for _, out := range outputs {
		if err := w.deliver(ctx, out, msg); err != nil {
			slog.Warn("delivery failed", "messageID", msg.ID, "output", out.ID, "err", err)
			success = false
		}
	}

	now := time.Now().UTC()
	if success {
		if err := ack(); err != nil {
			slog.Warn("ack failed", "messageID", msg.ID, "err", err)
		}
		if err := w.repo.UpdateDeliveryState(ctx, msg.ID, domain.MessageStatusDelivered, msg.RetryCount, now); err != nil {
			slog.Error("failed to update delivery state to delivered", "messageID", msg.ID, "err", err)
		}
	} else {
		if err := nack(); err != nil {
			slog.Warn("nack failed", "messageID", msg.ID, "err", err)
		}
		if err := w.repo.UpdateDeliveryState(ctx, msg.ID, domain.MessageStatusFailed, msg.RetryCount+1, now); err != nil {
			slog.Error("failed to update delivery state to failed", "messageID", msg.ID, "err", err)
		}
	}
	return nil
}

func (w *RelayWorker) deliver(ctx context.Context, out domain.Output, msg domain.Message) error {
	sender, err := w.registry.Get(out.Type)
	if err != nil {
		return fmt.Errorf("get sender: %w", err)
	}
	retryCount, delayMs := out.RetryCount, out.RetryDelayMs
	if retryCount <= 0 {
		retryCount = 3
	}
	if delayMs <= 0 {
		delayMs = 1000
	}
	var lastErr error
	for i := range retryCount {
		if err := sender.Send(ctx, out, msg); err == nil {
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

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
		if err := w.repo.UpdateDeliveryState(ctx, alert.ID, domain.AlertStatusDelivered, alert.RetryCount, now); err != nil {
			slog.Error("failed to update delivery state to delivered", "alertID", alert.ID, "err", err)
		}
	} else {
		if err := nack(); err != nil {
			slog.Warn("nack failed", "alertID", alert.ID, "err", err)
		}
		if err := w.repo.UpdateDeliveryState(ctx, alert.ID, domain.AlertStatusFailed, alert.RetryCount+1, now); err != nil {
			slog.Error("failed to update delivery state to failed", "alertID", alert.ID, "err", err)
		}
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

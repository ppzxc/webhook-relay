package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"relaybox/internal/application/port/output"
	"relaybox/internal/domain"
)

type RelayWorker struct {
	queue        output.MessageQueue
	repo         output.MessageRepository
	ruleReader   output.RuleConfigReader
	registry     output.OutputRegistry
	exprRegistry output.ExpressionEngineRegistry
	cfg          RelayWorkerConfig
	wg           sync.WaitGroup
}

func NewRelayWorker(
	queue output.MessageQueue,
	repo output.MessageRepository,
	ruleReader output.RuleConfigReader,
	registry output.OutputRegistry,
	exprRegistry output.ExpressionEngineRegistry,
	cfg RelayWorkerConfig,
) *RelayWorker {
	return &RelayWorker{
		queue: queue, repo: repo, ruleReader: ruleReader,
		registry: registry, exprRegistry: exprRegistry,
		cfg: cfg.withDefaults(),
	}
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
				case <-time.After(w.cfg.PollBackoff):
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

	rule, outputs, err := w.ruleReader.GetRule(ctx, string(msg.Input))
	if err != nil {
		_ = nack()
		return fmt.Errorf("get rule: %w", err)
	}

	// Build evaluation data from message
	data := buildEvalData(msg)

	// Get expression engine
	engine, err := w.getEngine(rule.Engine)
	if err != nil {
		_ = nack()
		return fmt.Errorf("get engine: %w", err)
	}

	// 1. Filter: skip if filter expression evaluates to false
	if rule.Filter != "" {
		pass, err := engine.EvaluateBool(rule.Filter, data)
		if err != nil {
			slog.Warn("filter evaluation failed", "messageID", msg.ID, "err", err)
			_ = nack()
			return err
		}
		if !pass {
			_ = ack() // message processed but filtered out
			return nil
		}
	}

	// 2. Mapping: evaluate all mapping expressions against the original data (parallel semantics).
	// All expressions see the same pre-mapping state; cross-key dependencies are not supported.
	mappedData := copyMap(data)
	for key, expr := range rule.Mapping {
		val, err := engine.Evaluate(expr, data)
		if err != nil {
			slog.Warn("mapping evaluation failed", "messageID", msg.ID, "key", key, "err", err)
			continue
		}
		mappedData[key] = val
	}

	// 3. Routing: evaluate conditions to determine which outputs to use
	var routedOutputs []domain.Output
	if len(rule.Routing) == 0 {
		// No routing conditions = send to all outputs
		routedOutputs = outputs
	} else {
		outputsByID := make(map[string]domain.Output, len(outputs))
		for _, o := range outputs {
			outputsByID[o.ID] = o
		}
		for _, rc := range rule.Routing {
			match, err := engine.EvaluateBool(rc.Condition, mappedData)
			if err != nil {
				slog.Warn("routing condition failed", "messageID", msg.ID, "condition", rc.Condition, "err", err)
				continue
			}
			if match {
				for _, oid := range rc.OutputIDs {
					if o, ok := outputsByID[oid]; ok {
						routedOutputs = append(routedOutputs, o)
					}
				}
			}
		}
	}

	// Deduplicate outputs to prevent double-sending when multiple routing conditions match the same output
	seen := make(map[string]struct{}, len(routedOutputs))
	deduped := routedOutputs[:0]
	for _, o := range routedOutputs {
		if _, ok := seen[o.ID]; !ok {
			seen[o.ID] = struct{}{}
			deduped = append(deduped, o)
		}
	}
	routedOutputs = deduped

	// 4. Deliver to each routed output
	success := true
	for _, out := range routedOutputs {
		payload, err := w.buildPayload(engine, out.Template, mappedData)
		if err != nil {
			slog.Warn("payload build failed", "messageID", msg.ID, "output", out.ID, "err", err)
			success = false
			continue
		}
		if err := w.deliver(ctx, out, payload); err != nil {
			slog.Warn("delivery failed", "messageID", msg.ID, "output", out.ID, "err", err)
			success = false
		}
	}

	now := time.Now().UTC()
	if success {
		if msg.Status.CanTransitionTo(domain.MessageStatusDelivered) {
			if err := w.repo.UpdateDeliveryState(ctx, msg.ID, domain.MessageStatusDelivered, msg.RetryCount, now); err != nil {
				slog.Error("failed to update delivery state to delivered", "messageID", msg.ID, "err", err)
			}
		} else {
			slog.Warn("invalid status transition, skipping update", "messageID", msg.ID, "from", msg.Status, "to", domain.MessageStatusDelivered)
		}
		if err := ack(); err != nil {
			slog.Warn("ack failed", "messageID", msg.ID, "err", err)
		}
	} else {
		if msg.Status.CanTransitionTo(domain.MessageStatusFailed) {
			if err := w.repo.UpdateDeliveryState(ctx, msg.ID, domain.MessageStatusFailed, msg.RetryCount+1, now); err != nil {
				slog.Error("failed to update delivery state to failed", "messageID", msg.ID, "err", err)
			}
		} else {
			slog.Warn("invalid status transition, skipping update", "messageID", msg.ID, "from", msg.Status, "to", domain.MessageStatusFailed)
		}
		if err := nack(); err != nil {
			slog.Warn("nack failed", "messageID", msg.ID, "err", err)
		}
	}
	return nil
}

func (w *RelayWorker) deliver(ctx context.Context, out domain.Output, payload []byte) error {
	sender, err := w.registry.Get(out.Type)
	if err != nil {
		return fmt.Errorf("get sender: %w", err)
	}
	retryCount, delayMs := out.RetryCount, out.RetryDelayMs
	if retryCount <= 0 {
		retryCount = w.cfg.DefaultRetryCount
	}
	if delayMs <= 0 {
		delayMs = w.cfg.DefaultRetryDelayMs
	}
	var lastErr error
	for i := range retryCount {
		if err := sender.Send(ctx, out, payload); err == nil {
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

var builtinEvalKeys = map[string]struct{}{
	"id": {}, "input": {}, "payload": {}, "createdAt": {}, "status": {},
}

func buildEvalData(msg domain.Message) map[string]any {
	data := map[string]any{
		"id":        msg.ID,
		"input":     string(msg.Input),
		"payload":   string(msg.Payload),
		"createdAt": msg.CreatedAt.Format(time.RFC3339),
		"status":    string(msg.Status),
	}
	// Merge ParsedData fields, skipping any key that would overwrite a builtin.
	for k, v := range msg.ParsedData {
		if _, reserved := builtinEvalKeys[k]; !reserved {
			data[k] = v
		}
	}
	return data
}

func (w *RelayWorker) buildPayload(engine output.ExpressionEngine, template map[string]string, data map[string]any) ([]byte, error) {
	if len(template) == 0 {
		// No template = use raw payload
		if raw, ok := data["payload"].(string); ok {
			return []byte(raw), nil
		}
		return []byte("{}"), nil
	}
	result := make(map[string]any, len(template))
	for key, expr := range template {
		val, err := engine.Evaluate(expr, data)
		if err != nil {
			return nil, fmt.Errorf("template key %q: %w", key, err)
		}
		setNested(result, key, val)
	}
	return json.Marshal(result)
}

// setNested writes val into m using dot-notation key as a path.
// "a.b.c" creates m["a"]["b"]["c"] = val.
// Keys without dots behave identically to m[key] = val.
func setNested(m map[string]any, key string, val any) {
	parts := strings.Split(key, ".")
	for _, p := range parts[:len(parts)-1] {
		child, ok := m[p].(map[string]any)
		if !ok {
			child = make(map[string]any)
			m[p] = child
		}
		m = child
	}
	m[parts[len(parts)-1]] = val
}

func (w *RelayWorker) getEngine(engineType string) (output.ExpressionEngine, error) {
	if engineType == "" {
		eng := w.exprRegistry.Default()
		if eng == nil {
			return nil, fmt.Errorf("no default expression engine registered")
		}
		return eng, nil
	}
	return w.exprRegistry.Get(engineType)
}

func copyMap(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

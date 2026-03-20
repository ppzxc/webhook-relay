package filequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	recoverOrphans(dir)
	return &Queue{dir: dir}, nil
}

// recoverOrphans는 프로세스 크래시로 잔류한 .json.processing 파일을 .json으로 복구한다.
// at-least-once 보장을 위해 New() 호출 시 항상 실행된다.
func recoverOrphans(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json.processing") {
			proc := filepath.Join(dir, e.Name())
			orig := strings.TrimSuffix(proc, ".processing")
			if err := os.Rename(proc, orig); err != nil {
				slog.Warn("failed to recover orphan file", "file", proc, "err", err)
			}
		}
	}
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

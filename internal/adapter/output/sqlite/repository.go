package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
	"relaybox/internal/adapter/output/sqlite/db"
	"relaybox/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ interface {
	Save(context.Context, domain.Message) error
} = (*Repository)(nil)

type Repository struct {
	queries *db.Queries
	sqlDB   *sql.DB
}

func New(dsn string) (*Repository, error) {
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := sqlDB.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Repository{queries: db.New(sqlDB), sqlDB: sqlDB}, nil
}

func (r *Repository) Close() error { return r.sqlDB.Close() }

func (r *Repository) Save(ctx context.Context, m domain.Message) error {
	err := r.queries.InsertMessage(ctx, db.InsertMessageParams{
		ID:         m.ID,
		Version:    int64(m.Version),
		Input:      string(m.Input),
		Payload:    []byte(m.Payload),
		CreatedAt:  m.CreatedAt.UTC(),
		Status:     string(m.Status),
		RetryCount: int64(m.RetryCount),
	})
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	return nil
}

func (r *Repository) UpdateDeliveryState(ctx context.Context, id string, status domain.MessageStatus, retryCount int, lastAttemptAt time.Time) error {
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

func (r *Repository) FindByID(ctx context.Context, id string) (domain.Message, error) {
	row, err := r.queries.GetMessageByID(ctx, id)
	if err != nil {
		return domain.Message{}, fmt.Errorf("find message %q: %w", id, err)
	}
	return toMessage(row), nil
}

func (r *Repository) FindByInput(ctx context.Context, inputID string, limit, offset int) ([]domain.Message, error) {
	rows, err := r.queries.ListMessagesByInput(ctx, db.ListMessagesByInputParams{
		Input: inputID, Limit: int64(limit), Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	messages := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessage(row))
	}
	return messages, nil
}

func toMessage(row db.Message) domain.Message {
	m := domain.Message{
		ID:         row.ID,
		Version:    int(row.Version),
		Input:      domain.InputType(row.Input),
		Payload:    domain.RawPayload(row.Payload),
		CreatedAt:  row.CreatedAt,
		Status:     domain.MessageStatus(row.Status),
		RetryCount: int(row.RetryCount),
	}
	if row.LastAttemptAt.Valid {
		t := row.LastAttemptAt.Time
		m.LastAttemptAt = &t
	}
	return m
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    version         INTEGER NOT NULL DEFAULT 1,
    input           TEXT NOT NULL,
    payload         BLOB NOT NULL,
    created_at      DATETIME NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_attempt_at DATETIME
);`

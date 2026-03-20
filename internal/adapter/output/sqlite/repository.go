package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"webhook-relay/internal/adapter/output/sqlite/db"
	"webhook-relay/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ interface {
	Save(context.Context, domain.Alert) error
} = (*Repository)(nil)

type Repository struct {
	queries *db.Queries
	sqlDB   *sql.DB
}

func New(dsn string) (*Repository, error) {
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := sqlDB.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Repository{queries: db.New(sqlDB), sqlDB: sqlDB}, nil
}

func (r *Repository) Close() error { return r.sqlDB.Close() }

func (r *Repository) Save(ctx context.Context, a domain.Alert) error {
	err := r.queries.InsertAlert(ctx, db.InsertAlertParams{
		ID:         a.ID,
		Version:    int64(a.Version),
		Source:     string(a.Source),
		Payload:    []byte(a.Payload),
		CreatedAt:  a.CreatedAt.UTC(),
		Status:     string(a.Status),
		RetryCount: int64(a.RetryCount),
	})
	if err != nil {
		return fmt.Errorf("save alert: %w", err)
	}
	return nil
}

func (r *Repository) UpdateDeliveryState(ctx context.Context, id string, status domain.AlertStatus, retryCount int, lastAttemptAt time.Time) error {
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

func (r *Repository) FindByID(ctx context.Context, id string) (domain.Alert, error) {
	row, err := r.queries.GetAlertByID(ctx, id)
	if err != nil {
		return domain.Alert{}, fmt.Errorf("find alert %q: %w", id, err)
	}
	return toAlert(row), nil
}

func (r *Repository) FindBySource(ctx context.Context, sourceID string, limit, offset int) ([]domain.Alert, error) {
	rows, err := r.queries.ListAlertsBySource(ctx, db.ListAlertsBySourceParams{
		Source: sourceID, Limit: int64(limit), Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	alerts := make([]domain.Alert, 0, len(rows))
	for _, row := range rows {
		alerts = append(alerts, toAlert(row))
	}
	return alerts, nil
}

func toAlert(row db.Alert) domain.Alert {
	a := domain.Alert{
		ID:         row.ID,
		Version:    int(row.Version),
		Source:     domain.SourceType(row.Source),
		Payload:    domain.RawPayload(row.Payload),
		CreatedAt:  row.CreatedAt,
		Status:     domain.AlertStatus(row.Status),
		RetryCount: int(row.RetryCount),
	}
	if row.LastAttemptAt.Valid {
		t := row.LastAttemptAt.Time
		a.LastAttemptAt = &t
	}
	return a
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS alerts (
    id              TEXT PRIMARY KEY,
    version         INTEGER NOT NULL DEFAULT 1,
    source          TEXT NOT NULL,
    payload         BLOB NOT NULL,
    created_at      DATETIME NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_attempt_at DATETIME
);`

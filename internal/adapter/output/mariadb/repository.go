package mariadb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"relaybox/internal/adapter/output/mariadb/db"
	output "relaybox/internal/application/port/output"
	"relaybox/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ output.MessageRepository = (*Repository)(nil)

// Config holds MariaDB connection and pool settings.
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// Repository implements port/output.MessageRepository for MariaDB.
type Repository struct {
	queries *db.Queries
	sqlDB   *sql.DB
}

// New opens a MariaDB connection, applies connection pool settings, and creates the schema.
func New(cfg Config) (*Repository, error) {
	dsn := ensureParseTime(cfg.DSN)

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mariadb: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping mariadb: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	if _, err := sqlDB.Exec(schemaSQL); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Repository{queries: db.New(sqlDB), sqlDB: sqlDB}, nil
}

func (r *Repository) Close() error { return r.sqlDB.Close() }

func (r *Repository) Save(ctx context.Context, m domain.Message) error {
	err := r.queries.InsertMessage(ctx, db.InsertMessageParams{
		ID:         m.ID,
		Version:    int32(m.Version),
		Input:      m.Input,
		Payload:    []byte(m.Payload),
		CreatedAt:  m.CreatedAt.UTC(),
		Status:     string(m.Status),
		RetryCount: int32(m.RetryCount),
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
		RetryCount:    int32(retryCount),
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
		Input:  inputID,
		Limit:  int32(limit),
		Offset: int32(offset),
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

func (r *Repository) DeleteOlderThan(ctx context.Context, cutoff time.Time, statuses []domain.MessageStatus) (int64, error) {
	var query string
	var args []any

	if len(statuses) == 0 {
		query = `DELETE FROM messages WHERE created_at < ?`
		args = []any{cutoff.UTC()}
	} else {
		placeholders := strings.Repeat("?,", len(statuses))
		placeholders = placeholders[:len(placeholders)-1]
		query = `DELETE FROM messages WHERE created_at < ? AND status IN (` + placeholders + `)`
		args = make([]any, 0, 1+len(statuses))
		args = append(args, cutoff.UTC())
		for _, s := range statuses {
			args = append(args, string(s))
		}
	}

	result, err := r.sqlDB.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete older than: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}

func toMessage(row db.Message) domain.Message {
	m := domain.Message{
		ID:         row.ID,
		Version:    int(row.Version),
		Input:      row.Input,
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

// ensureParseTime appends parseTime=true to the DSN if not already present.
// The go-sql-driver/mysql driver requires this to scan DATETIME into time.Time.
func ensureParseTime(dsn string) string {
	if strings.Contains(dsn, "parseTime=") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&parseTime=true"
	}
	return dsn + "?parseTime=true"
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS messages (
    id              VARCHAR(255) PRIMARY KEY,
    version         INT NOT NULL DEFAULT 1,
    input           VARCHAR(255) NOT NULL,
    payload         MEDIUMBLOB NOT NULL,
    created_at      DATETIME(6) NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    retry_count     INT NOT NULL DEFAULT 0,
    last_attempt_at DATETIME(6) NULL,
    INDEX idx_messages_input (input),
    INDEX idx_messages_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

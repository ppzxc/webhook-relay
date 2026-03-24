package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	output "relaybox/internal/application/port/output"
	"relaybox/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ output.MessageRepository = (*Repository)(nil)

type Repository struct {
	sqlDB     *sql.DB
	tableName string
}

func New(dsn, tableName string) (*Repository, error) {
	if tableName == "" {
		tableName = "messages"
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := sqlDB.Exec(buildSchemaSQL(tableName)); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Repository{sqlDB: sqlDB, tableName: tableName}, nil
}

func (r *Repository) Close() error { return r.sqlDB.Close() }

func (r *Repository) buildSQL(query string) string {
	return strings.ReplaceAll(query, "messages", r.tableName)
}

func (r *Repository) Save(ctx context.Context, m domain.Message) error {
	_, err := r.sqlDB.ExecContext(ctx, r.buildSQL(sqlInsertMessage),
		m.ID,
		int64(m.Version),
		m.Input,
		[]byte(m.Payload),
		m.CreatedAt.UTC(),
		string(m.Status),
		int64(m.RetryCount),
	)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	return nil
}

func (r *Repository) UpdateDeliveryState(ctx context.Context, id string, status domain.MessageStatus, retryCount int, lastAttemptAt time.Time) error {
	t := lastAttemptAt.UTC()
	_, err := r.sqlDB.ExecContext(ctx, r.buildSQL(sqlUpdateDeliveryState),
		string(status),
		int64(retryCount),
		sql.NullTime{Time: t, Valid: true},
		id,
	)
	if err != nil {
		return fmt.Errorf("update delivery state: %w", err)
	}
	return nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (domain.Message, error) {
	row := r.sqlDB.QueryRowContext(ctx, r.buildSQL(sqlGetMessageByID), id)
	m, err := scanMessage(row)
	if err != nil {
		return domain.Message{}, fmt.Errorf("find message %q: %w", id, err)
	}
	return m, nil
}

func (r *Repository) FindByInput(ctx context.Context, inputID string, limit, offset int) ([]domain.Message, error) {
	rows, err := r.sqlDB.QueryContext(ctx, r.buildSQL(sqlListMessagesByInput), inputID, int64(limit), int64(offset))
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	messages := make([]domain.Message, 0)
	for rows.Next() {
		m, err := scanMessageRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return messages, nil
}

func (r *Repository) DeleteOlderThan(ctx context.Context, cutoff time.Time, statuses []domain.MessageStatus) (int64, error) {
	var query string
	var args []any

	if len(statuses) == 0 {
		query = `DELETE FROM ` + r.tableName + ` WHERE created_at < ?`
		args = []any{cutoff.UTC()}
	} else {
		placeholders := strings.Repeat("?,", len(statuses))
		placeholders = placeholders[:len(placeholders)-1] // trailing comma 제거
		query = `DELETE FROM ` + r.tableName + ` WHERE created_at < ? AND status IN (` + placeholders + `)`
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

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMessage(row rowScanner) (domain.Message, error) {
	var m domain.Message
	var lastAttemptAt sql.NullTime
	var createdAt time.Time
	var status string
	var version, retryCount int64
	var payload []byte

	err := row.Scan(
		&m.ID,
		&version,
		&m.Input,
		&payload,
		&createdAt,
		&status,
		&retryCount,
		&lastAttemptAt,
	)
	if err != nil {
		return domain.Message{}, err
	}
	m.Version = int(version)
	m.Payload = domain.RawPayload(payload)
	m.CreatedAt = createdAt
	m.Status = domain.MessageStatus(status)
	m.RetryCount = int(retryCount)
	if lastAttemptAt.Valid {
		t := lastAttemptAt.Time
		m.LastAttemptAt = &t
	}
	return m, nil
}

func scanMessageRow(rows *sql.Rows) (domain.Message, error) {
	var m domain.Message
	var lastAttemptAt sql.NullTime
	var createdAt time.Time
	var status string
	var version, retryCount int64
	var payload []byte

	err := rows.Scan(
		&m.ID,
		&version,
		&m.Input,
		&payload,
		&createdAt,
		&status,
		&retryCount,
		&lastAttemptAt,
	)
	if err != nil {
		return domain.Message{}, err
	}
	m.Version = int(version)
	m.Payload = domain.RawPayload(payload)
	m.CreatedAt = createdAt
	m.Status = domain.MessageStatus(status)
	m.RetryCount = int(retryCount)
	if lastAttemptAt.Valid {
		t := lastAttemptAt.Time
		m.LastAttemptAt = &t
	}
	return m, nil
}

const (
	sqlInsertMessage = `
INSERT INTO messages (id, version, input, payload, created_at, status, retry_count)
VALUES (?, ?, ?, ?, ?, ?, ?)`

	sqlGetMessageByID = `
SELECT id, version, input, payload, created_at, status, retry_count, last_attempt_at
FROM messages WHERE id=?`

	sqlListMessagesByInput = `
SELECT id, version, input, payload, created_at, status, retry_count, last_attempt_at
FROM messages WHERE input=? ORDER BY created_at DESC LIMIT ? OFFSET ?`

	sqlUpdateDeliveryState = `
UPDATE messages SET status=?, retry_count=?, last_attempt_at=? WHERE id=?`
)

const schemaTemplate = `
CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    version         INTEGER NOT NULL DEFAULT 1,
    input           TEXT NOT NULL,
    payload         BLOB NOT NULL,
    created_at      DATETIME NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_attempt_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);`

func buildSchemaSQL(tableName string) string {
	s := strings.ReplaceAll(schemaTemplate, "messages", tableName)
	s = strings.ReplaceAll(s, "idx_"+tableName+"_created_at ON "+tableName, "idx_"+tableName+"_created_at ON "+tableName)
	return s
}

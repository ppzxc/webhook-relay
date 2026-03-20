-- name: InsertAlert :exec
INSERT INTO alerts (id, version, source, payload, created_at, status, retry_count)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateDeliveryState :exec
UPDATE alerts SET status=?, retry_count=?, last_attempt_at=? WHERE id=?;

-- name: GetAlertByID :one
SELECT id, version, source, payload, created_at, status, retry_count, last_attempt_at
FROM alerts WHERE id=?;

-- name: ListAlertsBySource :many
SELECT id, version, source, payload, created_at, status, retry_count, last_attempt_at
FROM alerts WHERE source=? ORDER BY created_at DESC LIMIT ? OFFSET ?;

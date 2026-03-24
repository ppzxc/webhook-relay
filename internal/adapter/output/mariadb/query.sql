-- name: InsertMessage :exec
INSERT INTO messages (id, version, input, payload, created_at, status, retry_count)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateDeliveryState :exec
UPDATE messages SET status=?, retry_count=?, last_attempt_at=? WHERE id=?;

-- name: GetMessageByID :one
SELECT id, version, input, payload, created_at, status, retry_count, last_attempt_at
FROM messages WHERE id=?;

-- name: ListMessagesByInput :many
SELECT id, version, input, payload, created_at, status, retry_count, last_attempt_at
FROM messages WHERE input=? ORDER BY created_at DESC LIMIT ? OFFSET ?;

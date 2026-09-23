-- name: CreateStatusChange :exec
INSERT INTO application_status_changes (application_id, status, changed_at)
VALUES (?, ?, ?);

-- Runs before the application update in the same transaction; it records a row only when the status actually changes.
-- name: RecordStatusChange :exec
INSERT INTO application_status_changes (application_id, status)
SELECT applications.id, sqlc.arg(status)
FROM applications
WHERE applications.id = sqlc.arg(id)
    AND applications.status <> sqlc.arg(status);

-- name: ListStatusChanges :many
SELECT application_id, status, changed_at
FROM application_status_changes
ORDER BY application_id, changed_at, id;

-- name: GetSettings :one
SELECT name, portal_check_days FROM settings WHERE id = 1;

-- name: UpdateSettings :exec
UPDATE settings SET name = ?, portal_check_days = ? WHERE id = 1;

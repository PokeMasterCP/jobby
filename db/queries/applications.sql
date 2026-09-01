-- name: CreateApplication :one
INSERT INTO applications (
    organization_id,
    role_title,
    posting_url,
    salary_min,
    salary_max,
    work_location,
    applied_at,
    last_checked_at,
    notes
) VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), ?)
RETURNING
    id,
    organization_id,
    role_title,
    status,
    posting_url,
    salary_min,
    salary_max,
    work_location,
    applied_at,
    status_changed_at,
    last_checked_at,
    notes,
    created_at,
    updated_at;

-- name: UpdateApplication :one
UPDATE applications
SET
    organization_id = sqlc.arg(organization_id),
    role_title = sqlc.arg(role_title),
    status_changed_at = CASE
        WHEN status <> sqlc.arg(status)
            THEN strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
        ELSE status_changed_at
    END,
    status = sqlc.arg(status),
    posting_url = sqlc.arg(posting_url),
    salary_min = sqlc.arg(salary_min),
    salary_max = sqlc.arg(salary_max),
    work_location = sqlc.arg(work_location),
    applied_at = sqlc.arg(applied_at),
    notes = sqlc.arg(notes),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = sqlc.arg(id)
RETURNING
    id,
    organization_id,
    role_title,
    status,
    posting_url,
    salary_min,
    salary_max,
    work_location,
    applied_at,
    status_changed_at,
    last_checked_at,
    notes,
    created_at,
    updated_at;

-- name: MarkApplicationChecked :one
UPDATE applications
SET
    last_checked_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?
RETURNING
    id,
    organization_id,
    role_title,
    status,
    posting_url,
    salary_min,
    salary_max,
    work_location,
    applied_at,
    status_changed_at,
    last_checked_at,
    notes,
    created_at,
    updated_at;

-- name: DeleteApplication :one
DELETE FROM applications
WHERE id = ?
RETURNING id;

-- name: ListApplications :many
SELECT
    applications.id,
    applications.organization_id,
    organizations.name AS organization_name,
    organizations.careers_url AS organization_careers_url,
    applications.role_title,
    applications.status,
    applications.posting_url,
    applications.salary_min,
    applications.salary_max,
    applications.work_location,
    applications.applied_at,
    applications.status_changed_at,
    applications.last_checked_at,
    applications.notes,
    applications.created_at,
    applications.updated_at
FROM applications
JOIN organizations ON organizations.id = applications.organization_id
ORDER BY applications.created_at DESC, applications.id DESC;

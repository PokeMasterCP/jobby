-- name: ListApplications :many
SELECT
    applications.id,
    applications.organization_id,
    organizations.name AS organization_name,
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

-- name: GetOrCreateOrganization :one
INSERT INTO organizations (name, careers_url)
VALUES (sqlc.arg(name), sqlc.narg(careers_url))
ON CONFLICT (name) DO UPDATE SET
    careers_url = COALESCE(excluded.careers_url, organizations.careers_url),
    updated_at = CASE
        WHEN excluded.careers_url IS NOT NULL
            THEN strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
        ELSE organizations.updated_at
    END
RETURNING
    id,
    name,
    careers_url,
    created_at,
    updated_at;

-- name: GetOrganization :one
SELECT
    id,
    name,
    careers_url,
    created_at,
    updated_at
FROM organizations
WHERE id = ?;

-- name: OrganizationNameInUse :one
SELECT COUNT(*)
FROM organizations
WHERE name = sqlc.arg(name)
    AND id <> sqlc.arg(id);

-- name: UpdateOrganization :one
UPDATE organizations
SET
    name = sqlc.arg(name),
    careers_url = sqlc.narg(careers_url),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = sqlc.arg(id)
RETURNING
    id,
    name,
    careers_url,
    created_at,
    updated_at;

-- name: GetOrCreateOrganization :one
INSERT INTO organizations (name)
VALUES (?)
ON CONFLICT (name) DO UPDATE SET name = organizations.name
RETURNING
    id,
    name,
    careers_url,
    created_at,
    updated_at;

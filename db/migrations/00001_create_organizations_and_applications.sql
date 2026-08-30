-- +goose Up
CREATE TABLE organizations (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
    careers_url TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE TABLE applications (
    id INTEGER PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role_title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'applied' CHECK (
        status IN (
            'applied',
            'in_contact',
            'accepted',
            'rejected_after_contact',
            'rejected_no_contact'
        )
    ),
    posting_url TEXT,
    salary_min INTEGER,
    salary_max INTEGER,
    work_location TEXT NOT NULL CHECK (work_location IN ('remote', 'local')),
    applied_at TEXT,
    status_changed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_checked_at TEXT,
    notes TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (
        (salary_min IS NULL AND salary_max IS NULL)
        OR (
            salary_min IS NOT NULL
            AND salary_max IS NOT NULL
            AND salary_min >= 0
            AND salary_max >= salary_min
        )
    )
) STRICT;

CREATE INDEX applications_organization_id_idx ON applications(organization_id);
CREATE INDEX applications_status_idx ON applications(status);

-- +goose Down
DROP TABLE applications;
DROP TABLE organizations;

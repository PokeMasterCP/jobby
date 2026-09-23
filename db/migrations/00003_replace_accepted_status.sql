-- SQLite cannot change a CHECK constraint in place, so the table is rebuilt
-- with the new status list and existing rows are copied across.

-- +goose Up
CREATE TABLE applications_new (
    id INTEGER PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role_title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'applied' CHECK (
        status IN (
            'applied',
            'in_contact',
            'offer',
            'rejected_after_contact',
            'rejected_no_contact',
            'withdrawn'
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

INSERT INTO applications_new (
    id, organization_id, role_title, status, posting_url, salary_min, salary_max,
    work_location, applied_at, status_changed_at, last_checked_at, notes, created_at, updated_at
)
SELECT
    id, organization_id, role_title,
    CASE status WHEN 'accepted' THEN 'offer' ELSE status END,
    posting_url, salary_min, salary_max,
    work_location, applied_at, status_changed_at, last_checked_at, notes, created_at, updated_at
FROM applications;

DROP TABLE applications;
ALTER TABLE applications_new RENAME TO applications;

CREATE INDEX applications_organization_id_idx ON applications(organization_id);
CREATE INDEX applications_status_idx ON applications(status);

-- +goose Down
-- The previous schema has no withdrawn status; those rows become rejected without contact.
CREATE TABLE applications_old (
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

INSERT INTO applications_old (
    id, organization_id, role_title, status, posting_url, salary_min, salary_max,
    work_location, applied_at, status_changed_at, last_checked_at, notes, created_at, updated_at
)
SELECT
    id, organization_id, role_title,
    CASE status
        WHEN 'offer' THEN 'accepted'
        WHEN 'withdrawn' THEN 'rejected_no_contact'
        ELSE status
    END,
    posting_url, salary_min, salary_max,
    work_location, applied_at, status_changed_at, last_checked_at, notes, created_at, updated_at
FROM applications;

DROP TABLE applications;
ALTER TABLE applications_old RENAME TO applications;

CREATE INDEX applications_organization_id_idx ON applications(organization_id);
CREATE INDEX applications_status_idx ON applications(status);

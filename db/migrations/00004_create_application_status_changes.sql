-- Earlier status changes were never recorded, so the backfill reconstructs what
-- the applications table still knows: every application started as applied when
-- it was created, and anything else reached its current status at status_changed_at.

-- +goose Up
CREATE TABLE application_status_changes (
    id INTEGER PRIMARY KEY,
    application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (
        status IN (
            'applied',
            'in_contact',
            'offer',
            'rejected_after_contact',
            'rejected_no_contact',
            'withdrawn'
        )
    ),
    changed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE INDEX application_status_changes_application_id_idx
    ON application_status_changes(application_id, changed_at);

INSERT INTO application_status_changes (application_id, status, changed_at)
SELECT id, 'applied', created_at
FROM applications;

INSERT INTO application_status_changes (application_id, status, changed_at)
SELECT id, status, status_changed_at
FROM applications
WHERE status <> 'applied';

-- +goose Down
DROP TABLE application_status_changes;

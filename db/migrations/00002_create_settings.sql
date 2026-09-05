-- +goose Up
CREATE TABLE settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    name TEXT NOT NULL DEFAULT '' CHECK (length(name) <= 100),
    portal_check_days INTEGER NOT NULL DEFAULT 7 CHECK (portal_check_days BETWEEN 1 AND 365)
);
INSERT INTO settings (id) VALUES (1);

-- +goose Down
DROP TABLE settings;

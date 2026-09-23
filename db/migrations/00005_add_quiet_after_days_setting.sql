-- +goose Up
ALTER TABLE settings
    ADD COLUMN quiet_after_days INTEGER NOT NULL DEFAULT 30 CHECK (quiet_after_days BETWEEN 1 AND 365);

-- +goose Down
ALTER TABLE settings DROP COLUMN quiet_after_days;

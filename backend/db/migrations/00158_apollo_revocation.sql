-- +goose Up
ALTER TABLE hr_apollo_connection ADD COLUMN revoking boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE hr_apollo_connection DROP COLUMN revoking;

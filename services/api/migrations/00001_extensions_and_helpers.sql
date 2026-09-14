-- +goose Up

-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS postgis;
-- +goose StatementEnd

-- Shared trigger that keeps updated_at honest. Relying on the application to
-- set it means any forgotten UPDATE silently produces a stale timestamp.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Guards append-only tables. A ledger that can be edited is not a ledger.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'table % is append-only, % is not allowed',
        TG_TABLE_NAME, TG_OP;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP FUNCTION IF EXISTS reject_mutation();
-- +goose StatementEnd

-- +goose StatementBegin
DROP FUNCTION IF EXISTS set_updated_at();
-- +goose StatementEnd

-- PostGIS is deliberately left installed: dropping an extension that other
-- databases in the cluster might share is not this migration's business.

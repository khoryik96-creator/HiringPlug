-- Server checks (1.52.0): each background process (the Go worker, the AI
-- worker) says every half minute that it is running and which version of
-- HyPlug it runs, so Settings > System can show a worker that stopped,
-- or one still on the old version after an update. One row per kind of
-- process (replicas share it). Not company data. The processes write it only
-- through server_process_seen(), as the AI worker reads nothing else; the API
-- only reads it.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE server_process (
  name        text PRIMARY KEY CHECK (name IN ('worker', 'ai-worker')),
  version     text NOT NULL CHECK (length(version) <= 40),
  started_at  timestamptz NOT NULL,
  seen_at     timestamptz NOT NULL
);
REVOKE INSERT, UPDATE, DELETE ON server_process FROM hiringplug_app;

-- +goose StatementBegin
CREATE FUNCTION server_process_seen(p_name text, p_version text, p_started timestamptz) RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS $$
  INSERT INTO server_process (name, version, started_at, seen_at)
  VALUES (p_name, left(p_version, 40), p_started, now())
  ON CONFLICT (name) DO UPDATE
    SET version = EXCLUDED.version, started_at = EXCLUDED.started_at, seen_at = EXCLUDED.seen_at
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION server_process_seen(text, text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION server_process_seen(text, text, timestamptz) TO hiringplug_app, hiringplug_ai;

-- +goose Down
DROP FUNCTION IF EXISTS server_process_seen(text, text, timestamptz);
DROP TABLE IF EXISTS server_process;

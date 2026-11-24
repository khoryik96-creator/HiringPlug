-- aiusage module: the AI use ledger, each company's limit and prices, and
-- the months a limit warning was reached. Only this module touches these
-- tables.

-- One AI run's tokens. The same run (tenant, feature, source, run key)
-- recorded again changes nothing, so a result delivered twice counts once.
-- name: RecordUse :execrows
INSERT INTO ai_use (tenant_id, feature, user_id, source_id, run_key, input_tokens, output_tokens)
VALUES (app_current_tenant(), @feature, sqlc.narg(user_id), @source_id, @run_key, @input_tokens, @output_tokens)
ON CONFLICT (tenant_id, feature, source_id, run_key) DO NOTHING;

-- What was used since a time (the start of the month).
-- name: UseSince :one
SELECT count(*)::bigint AS uses,
       coalesce(sum(input_tokens), 0)::bigint AS input_tokens,
       coalesce(sum(output_tokens), 0)::bigint AS output_tokens
FROM ai_use WHERE used_at >= @since;

-- Use per month (in the agency's time zone) and feature, since a time.
-- name: UseByMonthAndFeature :many
SELECT to_char(used_at AT TIME ZONE @time_zone::text, 'YYYY-MM')::text AS month, feature,
       count(*)::bigint AS uses,
       coalesce(sum(input_tokens), 0)::bigint AS input_tokens,
       coalesce(sum(output_tokens), 0)::bigint AS output_tokens
FROM ai_use WHERE used_at >= @since
GROUP BY 1, 2;

-- Use per person (NULL: the system on its own) since a time.
-- name: UseByPerson :many
SELECT user_id, count(*)::bigint AS uses,
       coalesce(sum(input_tokens), 0)::bigint AS input_tokens,
       coalesce(sum(output_tokens), 0)::bigint AS output_tokens
FROM ai_use WHERE used_at >= @since
GROUP BY user_id;

-- The company's limit and prices (no row: the defaults).
-- name: GetSetting :one
SELECT limit_kind, limit_tokens, limit_money, currency, price_input_per_million, price_output_per_million,
       warn_percent, version, updated_at
FROM ai_usage_setting WHERE tenant_id = app_current_tenant();

-- The first save (version 0 loaded): no row when someone saved first.
-- name: InsertSetting :one
INSERT INTO ai_usage_setting (tenant_id, limit_kind, limit_tokens, limit_money, currency,
  price_input_per_million, price_output_per_million, warn_percent, updated_by)
VALUES (app_current_tenant(), @limit_kind, sqlc.narg(limit_tokens), CAST(sqlc.narg(limit_money)::text AS numeric), @currency,
  CAST(sqlc.narg(price_input_per_million)::text AS numeric), CAST(sqlc.narg(price_output_per_million)::text AS numeric),
  @warn_percent, app_current_user())
ON CONFLICT (tenant_id) DO NOTHING
RETURNING version;

-- A later save: no row when the version loaded is not the latest.
-- name: UpdateSetting :one
UPDATE ai_usage_setting SET limit_kind = @limit_kind, limit_tokens = sqlc.narg(limit_tokens),
  limit_money = CAST(sqlc.narg(limit_money)::text AS numeric), currency = @currency,
  price_input_per_million = CAST(sqlc.narg(price_input_per_million)::text AS numeric),
  price_output_per_million = CAST(sqlc.narg(price_output_per_million)::text AS numeric),
  warn_percent = @warn_percent, version = version + 1, updated_by = app_current_user(), updated_at = now()
WHERE tenant_id = app_current_tenant() AND version = @version
RETURNING version;

-- A month's use reached a level (the warning percent or 100): true the first
-- time only. A month has one warning, so a warning percent changed later in
-- the month does not warn again.
-- name: ReachLevel :execrows
INSERT INTO ai_usage_alert (tenant_id, month, level)
SELECT app_current_tenant(), @month::date, @level::smallint
WHERE @level::smallint = 100 OR NOT EXISTS (
  SELECT 1 FROM ai_usage_alert WHERE tenant_id = app_current_tenant() AND month = @month::date AND level < 100)
ON CONFLICT (tenant_id, month, level) DO NOTHING;

-- The levels a month's use reached, and when.
-- name: LevelsReached :many
SELECT level, reached_at FROM ai_usage_alert WHERE month = @month::date ORDER BY level;

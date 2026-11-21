-- +goose Up
-- The CV index stays owned by documents. This read surface preserves its
-- tenant and candidate policies and lets the planner use the index in a join.
-- +goose StatementBegin
CREATE FUNCTION app_cv_candidate_matches(search text) RETURNS TABLE(candidate_id text)
LANGUAGE sql STABLE STRICT SECURITY INVOKER AS $$
  SELECT DISTINCT s.candidate_id FROM cv_search s
  WHERE s.tenant_id = app_current_tenant() AND s.terms @@ to_tsquery('english', search)
$$;

-- Normalize each sort to its own key without putting salary in a page token.
-- The first component of a numeric key keeps unknown values at the end.
CREATE FUNCTION app_candidate_order_key(mode text, name text, notice int, unit text,
  current_currency text, current_amount numeric, current_period text,
  expected_currency text, expected_min numeric, expected_period text)
RETURNS jsonb LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  WITH k AS (
    SELECT
      CASE WHEN mode LIKE 'current_salary%' THEN current_currency ELSE expected_currency END AS currency,
      CASE WHEN mode LIKE 'current_salary%' THEN current_amount ELSE expected_min END AS amount,
      CASE WHEN mode LIKE 'current_salary%' THEN current_period ELSE expected_period END AS period
  )
  SELECT CASE
    WHEN mode IN ('name', 'name_desc') THEN to_jsonb(name)
    WHEN mode IN ('notice', 'notice_desc') THEN jsonb_build_array(notice IS NULL,
      notice * CASE unit WHEN 'day' THEN 1 WHEN 'week' THEN 7 ELSE 30 END * CASE WHEN mode = 'notice_desc' THEN -1 ELSE 1 END)
    ELSE jsonb_build_array(currency IS NULL, CASE currency WHEN 'MYR' THEN '' ELSE currency END, amount IS NULL,
      amount * CASE period WHEN 'year' THEN 1.0 / 12 WHEN 'day' THEN 22 WHEN 'hour' THEN 176 ELSE 1 END *
      CASE WHEN mode LIKE '%_desc' THEN -1 ELSE 1 END)
  END FROM k
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION app_candidate_order_key(text, text, int, text, text, numeric, text, text, numeric, text);
DROP FUNCTION app_cv_candidate_matches(text);

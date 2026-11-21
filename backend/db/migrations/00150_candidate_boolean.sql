-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION app_candidate_boolean_in(plan jsonb, source tsvector)
RETURNS boolean LANGUAGE plpgsql IMMUTABLE STRICT AS $$
BEGIN
  IF jsonb_typeof(plan) = 'string' THEN
    RETURN source @@ to_tsquery('english', plan #>> '{}');
  ELSIF jsonb_typeof(plan) = 'boolean' THEN
    RETURN plan::boolean;
  END IF;

  CASE plan->>0
    WHEN 'and' THEN
      RETURN app_candidate_boolean_in(plan->1, source)
         AND app_candidate_boolean_in(plan->2, source);
    WHEN 'or' THEN
      RETURN app_candidate_boolean_in(plan->1, source)
          OR app_candidate_boolean_in(plan->2, source);
    ELSE RETURN false;
  END CASE;
END;
$$;

-- Resolve each NOT once across the candidate's sources before checking for
-- a match. In particular, nested NOTs cannot repeatedly scan the same CVs.
CREATE FUNCTION app_candidate_boolean_resolve(plan jsonb, sources tsvector[])
RETURNS jsonb LANGUAGE plpgsql IMMUTABLE STRICT AS $$
DECLARE
  operand jsonb;
BEGIN
  IF jsonb_typeof(plan) <> 'array' THEN RETURN plan; END IF;
  operand := app_candidate_boolean_resolve(plan->1, sources);
  IF plan->>0 = 'not' THEN
    RETURN to_jsonb(NOT EXISTS (
      SELECT 1 FROM unnest(sources) AS document(terms)
      WHERE app_candidate_boolean_in(operand, document.terms)
    ));
  END IF;
  RETURN jsonb_build_array(plan->0, operand, app_candidate_boolean_resolve(plan->2, sources));
END;
$$;

-- The documents index supplies independent sources. The function runs as
-- the caller, so the existing tenant and candidate RLS policies still apply.
CREATE FUNCTION app_candidate_boolean_matches(id text, profile tsvector, plan jsonb)
RETURNS integer LANGUAGE plpgsql STABLE STRICT SECURITY INVOKER AS $$
DECLARE
  cv_sources tsvector[];
  resolved jsonb;
  hits integer := 0;
BEGIN
  SELECT coalesce(array_agg(terms), ARRAY[]::tsvector[]) INTO cv_sources
  FROM cv_search WHERE tenant_id = app_current_tenant() AND candidate_id = id;
  resolved := app_candidate_boolean_resolve(plan, array_prepend(profile, cv_sources));
  IF app_candidate_boolean_in(resolved, profile) THEN hits := 1; END IF;
  IF EXISTS (SELECT 1 FROM unnest(cv_sources) AS cv(terms)
             WHERE app_candidate_boolean_in(resolved, cv.terms)) THEN
    hits := hits + 2;
  END IF;
  RETURN hits;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION app_candidate_boolean_matches(text, tsvector, jsonb);
DROP FUNCTION app_candidate_boolean_resolve(jsonb, tsvector[]);
DROP FUNCTION app_candidate_boolean_in(jsonb, tsvector);

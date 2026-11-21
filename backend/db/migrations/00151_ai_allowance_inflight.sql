-- +goose Up
CREATE INDEX job_ai_allowance_active_idx ON job (tenant_id, kind) WHERE status IN ('pending', 'running');

-- A Python job and its core result change over in one transaction. Include
-- both sides so a charge remains reserved until Go records actual usage.
-- +goose StatementBegin
CREATE FUNCTION job_ai_inflight()
RETURNS bigint LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT count(*) FROM job WHERE tenant_id = app_current_tenant() AND status IN ('pending', 'running')
  AND (
    (queue = 'ai' AND kind IN ('assistant.ask', 'assistant.email', 'match.score', 'talent.rank',
      'jd.blind', 'cv.format_studio', 'interview_prep.write', 'company.industry'))
    OR (queue = 'ai' AND kind IN ('job.read_jd', 'document.parse_cv') AND payload->>'useAi' = 'true')
    OR (queue = 'core' AND kind IN ('assistant.answered', 'assistant.emailed', 'match.scored', 'talent.ranked',
      'jd.blinded', 'cv.studio_formatted', 'interview_prep.written', 'company.industried'))
    OR (queue = 'core' AND kind IN ('job.jd_read', 'document.cv_parsed') AND
        coalesce(payload->>'aiTokens', '0') NOT IN ('0', 'null'))
  )
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION job_ai_inflight();
DROP INDEX job_ai_allowance_active_idx;

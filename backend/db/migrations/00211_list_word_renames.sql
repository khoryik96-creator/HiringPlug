-- settings (1.60.1): a word renamed in the specialization, IT skills or
-- certifications list is renamed on every candidate of the company who has it
-- (whoever owns them, binned ones too) and in everyone's favourites, not only
-- on the rows the renaming admin may see. Only those who manage Settings may
-- run these; each checks the company and the permission itself.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
-- The words with from_word (ignoring case) changed to to_word, each word once
-- (ignoring case) where it came first.
CREATE FUNCTION app_rename_word(words text[], from_word text, to_word text) RETURNS text[]
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT coalesce(array_agg(w ORDER BY i), '{}') FROM (
    SELECT DISTINCT ON (lower(w)) w, i FROM (
      SELECT CASE WHEN lower(s) = lower(from_word) THEN to_word ELSE s END AS w, i
      FROM unnest(words) WITH ORDINALITY AS u(s, i)
    ) renamed ORDER BY lower(w), i
  ) once
$$;

-- Renames a word in one field (skills, certifications or specialization) of
-- every candidate of the company who has it, and gives their ids.
CREATE FUNCTION app_rename_candidate_word(p_field text, p_from text, p_to text) RETURNS SETOF text
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  IF NOT app_has_permission('settings:manage') THEN
    RAISE EXCEPTION 'settings:manage is needed to rename a word' USING ERRCODE = '42501';
  END IF;
  IF p_field = 'skills' THEN
    RETURN QUERY UPDATE candidate SET skills = app_rename_word(skills, p_from, p_to), version = version + 1, updated_at = now()
      WHERE tenant_id = app_current_tenant() AND app_lower_array(skills) @> ARRAY[lower(p_from)] RETURNING id;
  ELSIF p_field = 'certifications' THEN
    RETURN QUERY UPDATE candidate SET certifications = app_rename_word(certifications, p_from, p_to), version = version + 1, updated_at = now()
      WHERE tenant_id = app_current_tenant() AND app_lower_array(certifications) @> ARRAY[lower(p_from)] RETURNING id;
  ELSIF p_field = 'specialization' THEN
    RETURN QUERY UPDATE candidate SET specialization = p_to, version = version + 1, updated_at = now()
      WHERE tenant_id = app_current_tenant() AND lower(specialization) = lower(p_from) RETURNING id;
  ELSE
    RAISE EXCEPTION 'no word field %', p_field;
  END IF;
END
$$;
REVOKE ALL ON FUNCTION app_rename_candidate_word(text, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_rename_candidate_word(text, text, text) TO hiringplug_app;

-- Renames a word in one kind of everyone's favourites (itSkill, certification
-- or specialization), and says how many people's changed.
CREATE FUNCTION app_rename_favourite(p_kind text, p_from text, p_to text) RETURNS integer
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE n integer;
BEGIN
  IF NOT app_has_permission('settings:manage') THEN
    RAISE EXCEPTION 'settings:manage is needed to rename a word' USING ERRCODE = '42501';
  END IF;
  UPDATE user_preference SET favourites = jsonb_set(favourites, ARRAY[p_kind],
      to_jsonb(app_rename_word(ARRAY(SELECT jsonb_array_elements_text(favourites -> p_kind)), p_from, p_to))),
    updated_at = now()
  WHERE tenant_id = app_current_tenant() AND jsonb_typeof(favourites -> p_kind) = 'array'
    AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(favourites -> p_kind) f WHERE lower(f) = lower(p_from));
  GET DIAGNOSTICS n = ROW_COUNT;
  RETURN n;
END
$$;
REVOKE ALL ON FUNCTION app_rename_favourite(text, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_rename_favourite(text, text, text) TO hiringplug_app;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS app_rename_favourite(text, text, text);
DROP FUNCTION IF EXISTS app_rename_candidate_word(text, text, text);
DROP FUNCTION IF EXISTS app_rename_word(text[], text, text);

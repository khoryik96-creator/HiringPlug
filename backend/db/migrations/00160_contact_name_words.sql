-- +goose Up
-- The words of a person's name, to find the same person under another way of
-- writing it (Find HR checks the people it finds against the contacts): lower
-- case, without accents, punctuation or what is in brackets ("(She/Her)");
-- each word once, sorted; without single letters, numbers, titles (Dato', Dr,
-- Hj), bin/binti and credentials (CHRP, SHRM-CP); the usual short forms
-- written out (Mohd, Md, Muhd: muhammad; Abd: abdul). Other functions are
-- named with their schema: a restore runs with an empty search_path.
-- +goose StatementBegin
CREATE FUNCTION app_person_name_words(value text) RETURNS text[]
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT coalesce(array_agg(DISTINCT word ORDER BY word), '{}'::text[])
  FROM (
    SELECT CASE
             WHEN w IN ('mohd', 'muhd', 'md', 'mohamad', 'mohammad', 'muhamad', 'mohammed', 'mohamed', 'muhammed') THEN 'muhammad'
             WHEN w = 'abd' THEN 'abdul'
             ELSE w
           END AS word
    FROM regexp_split_to_table(
           regexp_replace(public.app_normalize_text(regexp_replace(coalesce(value, ''), '\([^)]*\)|\[[^]]*\]|\{[^}]*\}', ' ', 'g')), '[^[:alnum:]]+', ' ', 'g'),
           ' ') AS w
    WHERE length(w) > 1 AND w !~ '^[0-9]+$'
      AND w <> ALL (ARRAY['bin', 'binti', 'binte', 'bte', 'bt', 'anak', 'dato', 'datuk', 'datin', 'dr', 'mr', 'mrs', 'ms', 'miss', 'mdm',
        'madam', 'puan', 'pn', 'encik', 'cik', 'hj', 'haji', 'hjh', 'hajah', 'prof', 'ir', 'jr', 'sr', 'chrp', 'chrm', 'shrm', 'cp',
        'scp', 'phr', 'sphr', 'gphr', 'cipd', 'mcipd', 'cpa', 'acca', 'cfa', 'mba', 'phd', 'pmp'])
  ) words
$$;
-- +goose StatementEnd

-- Kept with every contact however it is saved (generated), and searchable.
ALTER TABLE contact ADD COLUMN name_words text[] NOT NULL
  GENERATED ALWAYS AS (app_person_name_words(first_name || ' ' || last_name)) STORED;
CREATE INDEX contact_name_words_idx ON contact USING gin (name_words);
-- A contact by LinkedIn profile, whatever the letter case or a final slash.
CREATE INDEX contact_linkedin_key_idx ON contact (tenant_id, lower(rtrim(linkedin_url, '/'))) WHERE linkedin_url IS NOT NULL;

-- +goose Down
DROP INDEX contact_linkedin_key_idx;
DROP INDEX contact_name_words_idx;
ALTER TABLE contact DROP COLUMN name_words;
DROP FUNCTION app_person_name_words(text);

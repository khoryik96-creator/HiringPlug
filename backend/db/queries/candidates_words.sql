-- Renaming a word in the company's specialization, IT skills or
-- certifications list (Settings) renames it on every candidate who has it,
-- ignoring case. A candidate who already has the new word keeps it once, where
-- it came first. The version goes up, so a form opened before shows a conflict
-- instead of putting the old word back.

-- name: RenameSkill :execrows
UPDATE candidate SET
  skills = ARRAY(
    SELECT w FROM (
      SELECT DISTINCT ON (lower(w)) w, i FROM (
        SELECT CASE WHEN lower(s) = lower(@from_word::text) THEN @to_word::text ELSE s END AS w, i
        FROM unnest(skills) WITH ORDINALITY AS u(s, i)
      ) renamed ORDER BY lower(w), i
    ) once ORDER BY i),
  version = version + 1
WHERE app_lower_array(skills) @> ARRAY[lower(@from_word::text)];

-- name: RenameCertification :execrows
UPDATE candidate SET
  certifications = ARRAY(
    SELECT w FROM (
      SELECT DISTINCT ON (lower(w)) w, i FROM (
        SELECT CASE WHEN lower(s) = lower(@from_word::text) THEN @to_word::text ELSE s END AS w, i
        FROM unnest(certifications) WITH ORDINALITY AS u(s, i)
      ) renamed ORDER BY lower(w), i
    ) once ORDER BY i),
  version = version + 1
WHERE app_lower_array(certifications) @> ARRAY[lower(@from_word::text)];

-- name: RenameSpecialization :execrows
UPDATE candidate SET specialization = @to_word::text, version = version + 1
WHERE lower(specialization) = lower(@from_word::text);

-- Binned candidates are renamed too (they come back with the new word):
-- the renames run with app.show_binned on, and it goes back off after.
-- name: ShowBinnedWords :exec
SELECT set_config('app.show_binned', @value::text, true);

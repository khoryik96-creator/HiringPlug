-- Renaming a word in the company's specialization, IT skills or
-- certifications list (Settings) renames it on every candidate who has it,
-- ignoring case: whoever owns them, binned ones too (app_rename_candidate_word,
-- which checks settings:manage). A candidate who already has the new word
-- keeps it once, where it came first. The version goes up, so a form opened
-- before shows a conflict instead of putting the old word back.

-- name: RenameCandidateWord :many
SELECT app_rename_candidate_word(@field::text, @from_word::text, @to_word::text)::text AS id;

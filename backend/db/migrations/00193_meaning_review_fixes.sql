-- Search by meaning, after its review (1.40.3):
-- meaning_role.current: whether a title is their job now. A past job with no
--   end year was taken for their job now; null for titles read before this
--   (then their job now when they have no end year, as before), and every
--   candidate is read again by the new reading rules.
-- meaning_sweep.read_count, total_count, counted_at: how far the company's
--   read-through has got, counted by the read-through itself (at the start of
--   each pass, and every few minutes during one), so a search no longer
--   counts the whole company on every request.
-- What was read of a candidate in the Bin is not used: their meaning rows are
--   visible only while the candidate is (in the user's scope and not in the
--   Bin). A CV moved to the Bin has its passages removed by the meaning module.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE meaning_role ADD COLUMN current boolean;
ALTER TABLE meaning_sweep ADD COLUMN read_count integer, ADD COLUMN total_count integer, ADD COLUMN counted_at timestamptz;

-- Whether the user may see this candidate now: in their scope and not in the
-- Bin (the candidate's own row-level security decides).
CREATE FUNCTION app_sees_candidate_now(p_id text) RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM candidate WHERE id = p_id)
$$;

DROP POLICY candidate_scope ON meaning_candidate;
CREATE POLICY candidate_scope ON meaning_candidate AS RESTRICTIVE FOR SELECT USING (app_sees_candidate_now(candidate_id));
DROP POLICY candidate_scope ON meaning_role;
CREATE POLICY candidate_scope ON meaning_role AS RESTRICTIVE FOR SELECT USING (app_sees_candidate_now(candidate_id));
DROP POLICY candidate_scope ON meaning_passage;
CREATE POLICY candidate_scope ON meaning_passage AS RESTRICTIVE FOR SELECT USING (app_sees_candidate_now(candidate_id));

-- +goose Down
DROP POLICY candidate_scope ON meaning_passage;
CREATE POLICY candidate_scope ON meaning_passage AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
DROP POLICY candidate_scope ON meaning_role;
CREATE POLICY candidate_scope ON meaning_role AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
DROP POLICY candidate_scope ON meaning_candidate;
CREATE POLICY candidate_scope ON meaning_candidate AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
DROP FUNCTION IF EXISTS app_sees_candidate_now(text);
ALTER TABLE meaning_sweep DROP COLUMN IF EXISTS counted_at, DROP COLUMN IF EXISTS total_count, DROP COLUMN IF EXISTS read_count;
ALTER TABLE meaning_role DROP COLUMN IF EXISTS current;

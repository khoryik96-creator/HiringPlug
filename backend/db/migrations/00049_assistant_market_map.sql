-- assistant_answer: market insights for a job (kind market_map).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE assistant_answer DROP CONSTRAINT assistant_answer_kind_check;
ALTER TABLE assistant_answer ADD CONSTRAINT assistant_answer_kind_check
  CHECK (kind IN ('summary', 'red_flags', 'fit', 'question', 'market_map'));

-- +goose Down
DELETE FROM assistant_answer WHERE kind = 'market_map';
ALTER TABLE assistant_answer DROP CONSTRAINT assistant_answer_kind_check;
ALTER TABLE assistant_answer ADD CONSTRAINT assistant_answer_kind_check
  CHECK (kind IN ('summary', 'red_flags', 'fit', 'question'));

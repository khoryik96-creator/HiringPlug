-- emails: invoices emailed before emails were marked as showing a fee
--   (fees_restricted) are marked now, so only roles with placements.fees:read
--   see them: automatic emails that show salary and carry an "Invoice ….pdf".
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
UPDATE email e SET fees_restricted = true
WHERE e.salary AND e.salary_restricted AND NOT e.fees_restricted
  AND EXISTS (SELECT 1 FROM email_attachment a WHERE a.email_id = e.id AND a.file_name LIKE 'Invoice %.pdf');

-- +goose Down
SELECT 1; -- marking them again would hide nothing more: nothing to undo

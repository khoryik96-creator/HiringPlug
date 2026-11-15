-- contact_mailbox_check: the latest check of a contact's email with their
-- mail server (does the mailbox exist?), one per contact. It applies to
-- `email` only: a changed email has no check until checked again. The made-up
-- control mailbox (control_id) is kept for 4 hours so a recheck asks about the
-- same one. No reply text from mail servers is stored.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE contact_mailbox_check (
  tenant_id          text NOT NULL REFERENCES tenant (id),
  contact_id         text NOT NULL,
  email              text NOT NULL CHECK (length(email) <= 320),
  status             text NOT NULL CHECK (status IN ('checking', 'accepted', 'catch_all', 'invalid', 'unknown')),
  reason             text CHECK (length(reason) <= 500),
  outcome            text CHECK (length(outcome) <= 40),
  provider           text CHECK (length(provider) <= 40),
  attempts           integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 10),
  control_id         text NOT NULL CHECK (control_id ~ '^[a-f0-9]{32}$'),
  control_expires_at timestamptz NOT NULL,
  rechecks           integer NOT NULL DEFAULT 0 CHECK (rechecks BETWEEN 0 AND 3),
  recheck_at         timestamptz,
  requested_by       text NOT NULL,
  requested_at       timestamptz NOT NULL DEFAULT now(),
  checked_at         timestamptz,
  PRIMARY KEY (tenant_id, contact_id),
  FOREIGN KEY (tenant_id, contact_id) REFERENCES contact (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE contact_mailbox_check ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contact_mailbox_check
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS contact_mailbox_check;

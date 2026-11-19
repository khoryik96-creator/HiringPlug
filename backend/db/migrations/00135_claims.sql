-- claims module: expense claims, one per receipt. The receipt is read by OCR
-- (Python AI worker) and fills in the fields still empty; the claimant
-- completes the rest and submits; approvers decide and mark them paid.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE claim (
  id            text PRIMARY KEY DEFAULT new_id('clm') CHECK (id ~ '^clm_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id     text NOT NULL REFERENCES tenant (id),
  user_id       text NOT NULL,
  status        text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted', 'approved', 'rejected', 'paid')),
  -- The receipt as uploaded, and how reading it went.
  file_name     text NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
  content_type  text NOT NULL,
  size_bytes    bigint NOT NULL CHECK (size_bytes > 0),
  sha256        text NOT NULL,
  storage_key   text NOT NULL,
  ocr_status    text NOT NULL DEFAULT 'reading' CHECK (ocr_status IN ('reading', 'done', 'failed')),
  ocr_error     text CHECK (length(ocr_error) <= 500),
  ocr_extractor text CHECK (length(ocr_extractor) <= 200),
  -- What is claimed: read from the receipt where it could be, then edited.
  receipt_date  date,
  amount        numeric(14, 2) CHECK (amount >= 0),
  currency      text NOT NULL DEFAULT 'MYR' CHECK (currency ~ '^[A-Z]{3}$'),
  merchant      text CHECK (length(merchant) <= 200),
  receipt_type  text CHECK (length(receipt_type) <= 100),  -- a claim_receipt_type option
  claim_type    text CHECK (length(claim_type) <= 100),    -- a claim_type option
  purpose       text CHECK (length(purpose) <= 500),
  -- The client (company or contact) or candidate it was for. No foreign key:
  -- the record belongs to another module (and a deleted one leaves the tag).
  link_kind     text CHECK (link_kind IN ('company', 'contact', 'candidate')),
  link_id       text CHECK (length(link_id) <= 40),
  -- Fields whose value came from the receipt and were not changed since.
  from_receipt  text[] NOT NULL DEFAULT '{}',
  claimed_at    timestamptz,
  reviewed_at   timestamptz,
  reviewed_by   text,
  review_note   text CHECK (length(review_note) <= 500),
  paid_at       timestamptz,
  version       integer NOT NULL DEFAULT 1,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  CHECK ((link_kind IS NULL) = (link_id IS NULL)),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id)
);
CREATE INDEX claim_user_idx ON claim (tenant_id, user_id, created_at DESC);
CREATE INDEX claim_status_idx ON claim (tenant_id, status, claimed_at DESC);
CREATE INDEX claim_link_idx ON claim (tenant_id, link_kind, link_id) WHERE link_id IS NOT NULL;
-- The same receipt (the same bytes) is claimed once by each person.
CREATE UNIQUE INDEX claim_receipt_once ON claim (tenant_id, user_id, sha256);

-- Every table holding tenant data has row-level security (a test checks this).
-- A person sees and changes their own claims; approvers (claims:approve) see
-- and decide everyone's. Only the claimant adds or deletes their claims.
ALTER TABLE claim ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON claim AS RESTRICTIVE
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY claim_select ON claim FOR SELECT
  USING (user_id = app_current_user() OR app_has_permission('claims:approve'));
CREATE POLICY claim_insert ON claim FOR INSERT
  WITH CHECK (user_id = app_current_user() AND app_has_permission('claims:create'));
CREATE POLICY claim_update ON claim FOR UPDATE
  USING (user_id = app_current_user() OR app_has_permission('claims:approve'))
  WITH CHECK (user_id = app_current_user() OR app_has_permission('claims:approve'));
CREATE POLICY claim_delete ON claim FOR DELETE
  USING (user_id = app_current_user());

-- +goose Down
DROP TABLE IF EXISTS claim;

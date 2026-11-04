-- privacy module (PDPA): each candidate's consent, and the requests they make
-- about their data. Nothing here deletes anything: people decide requests.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE candidate_consent (
  candidate_id text PRIMARY KEY CHECK (length(candidate_id) <= 40),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  version      integer NOT NULL DEFAULT 1,
  status       text NOT NULL CHECK (status IN ('given', 'withdrawn')),
  method       text NOT NULL CHECK (method IN ('careers_form', 'email', 'phone', 'in_person', 'document', 'request', 'other')),
  day          date NOT NULL,
  note         text CHECK (length(note) <= 1000),
  recorded_by  text NOT NULL,
  updated_at   timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX candidate_consent_status_idx ON candidate_consent (tenant_id, status, day DESC);

CREATE TABLE privacy_request (
  id           text PRIMARY KEY DEFAULT new_id('prq') CHECK (id ~ '^prq_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  version      integer NOT NULL DEFAULT 1,
  candidate_id text NOT NULL CHECK (length(candidate_id) <= 40),
  kind         text NOT NULL CHECK (kind IN ('access', 'correction', 'withdraw_consent', 'removal', 'other')),
  received_on  date NOT NULL,
  details      text CHECK (length(details) <= 2000),
  status       text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done', 'declined')),
  outcome      text CHECK (length(outcome) <= 2000),
  closed_by    text,
  closed_at    timestamptz,
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  CHECK ((status = 'open') = (closed_at IS NULL)),
  CHECK (status = 'open' OR outcome IS NOT NULL),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX privacy_request_candidate_idx ON privacy_request (tenant_id, candidate_id, created_at DESC);
CREATE INDEX privacy_request_open_idx ON privacy_request (tenant_id, status, received_on);

-- Every table holding tenant data has row-level security (a test checks this).
ALTER TABLE candidate_consent ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON candidate_consent
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE privacy_request ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON privacy_request
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS privacy_request;
DROP TABLE IF EXISTS candidate_consent;

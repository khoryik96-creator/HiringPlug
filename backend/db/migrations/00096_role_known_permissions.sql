-- A company's own settings for a role list what it may do. A permission
-- added in a later version was never offered to them, so it was simply
-- missing from those roles. Each setting now also keeps which permissions
-- existed when it was saved: one added since then follows the role's
-- default (given if the role has it by default) until the owner chooses.
-- Settings saved before this knew every permission that exists today.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE role_setting ADD COLUMN known_permissions text[];
UPDATE role_setting SET known_permissions = ARRAY[
  'aiproviders:manage', 'assistant:use', 'automations.tasks:read', 'automations.tasks:update', 
  'automations:approve', 'automations:manage', 'automations:read', 'candidates.compensation:read', 
  'candidates.compensation:update', 'candidates:create', 'candidates:merge', 'candidates:read', 
  'candidates:update', 'companies.documents:delete', 'companies:create', 'companies:read', 'companies:update', 
  'contacts:create', 'contacts:read', 'contacts:update', 'cvformats.templates:manage', 'cvformats:create', 
  'documents:create', 'documents:delete', 'documents:read', 'emails.apps:manage', 'emails.templates:create', 
  'emails.templates:manage', 'emails:read', 'emails:send', 'identity.roles:manage', 'identity.team:manage', 
  'intake:create', 'integrations:manage', 'interviews:feedback', 'interviews:read', 'interviews:schedule', 
  'jobs.documents:delete', 'jobs.salary:read', 'jobs.salary:update', 'jobs:create', 'jobs:read', 'jobs:update', 
  'leads:manage', 'leads:read', 'market:read', 'matching:create', 'matching:read', 'notes.activity:read', 
  'notes.sensitive:read', 'notes:create', 'notes:manage', 'notes:read', 'offers.salary:read', 
  'offers.salary:update', 'offers:approve', 'offers:manage', 'offers:read', 'pipeline:read', 'pipeline:update', 
  'placements.fees:read', 'placements.fees:update', 'placements:manage', 'placements:read', 'privacy:manage', 
  'privacy:read', 'privacy:record', 'reports:read', 'scheduling:use', 'settings.backups:manage', 
  'settings:manage', 'settings:read'
];
ALTER TABLE role_setting ALTER COLUMN known_permissions SET NOT NULL;

-- +goose Down
ALTER TABLE role_setting DROP COLUMN known_permissions;

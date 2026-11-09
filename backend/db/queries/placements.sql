-- Queries owned by the placements module. Run inside a tenant-scoped transaction.

-- name: InsertPlacement :one
INSERT INTO placement (tenant_id, application_id, recruiter_id, start_date, guarantee_days, created_by)
VALUES (app_current_tenant(), @application_id, sqlc.narg(recruiter_id), @start_date, @guarantee_days, app_current_user())
RETURNING *;

-- name: GetPlacement :one
SELECT * FROM placement WHERE id = @id;

-- name: ListPlacements :many
SELECT * FROM placement
WHERE (sqlc.narg(application_id)::text IS NULL OR application_id = sqlc.narg(application_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(invoice_status)::text IS NULL OR invoice_status = sqlc.narg(invoice_status))
  AND (sqlc.narg(start_from)::date IS NULL OR start_date >= sqlc.narg(start_from))
  AND (sqlc.narg(start_to)::date IS NULL OR start_date <= sqlc.narg(start_to))
ORDER BY start_date DESC, id DESC
LIMIT 500;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdatePlacement :one
UPDATE placement SET recruiter_id = sqlc.narg(recruiter_id), start_date = @start_date, guarantee_days = @guarantee_days,
  status = @status, left_on = sqlc.narg(left_on), status_note = sqlc.narg(status_note), replacement = @replacement,
  invoice_status = @invoice_status, invoice_number = sqlc.narg(invoice_number),
  invoiced_on = sqlc.narg(invoiced_on), paid_on = sqlc.narg(paid_on),
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- Visible only with placements.fees:read (row-level security).
-- name: PlacementMoney :many
SELECT * FROM placement_money WHERE placement_id = ANY(@ids::text[]);

-- Writable only with placements.fees:update (row-level security).
-- name: PutPlacementMoney :exec
INSERT INTO placement_money (placement_id, tenant_id, salary_amount, salary_currency, salary_period,
                             fee_percent, fee_amount, fee_currency, updated_by)
VALUES (@placement_id, app_current_tenant(), sqlc.narg(salary_amount)::text::numeric, sqlc.narg(salary_currency), sqlc.narg(salary_period),
        sqlc.narg(fee_percent)::text::numeric, sqlc.narg(fee_amount)::text::numeric, sqlc.narg(fee_currency), app_current_user())
ON CONFLICT (placement_id) DO UPDATE SET
  salary_amount = EXCLUDED.salary_amount, salary_currency = EXCLUDED.salary_currency, salary_period = EXCLUDED.salary_period,
  fee_percent = EXCLUDED.fee_percent, fee_amount = EXCLUDED.fee_amount, fee_currency = EXCLUDED.fee_currency,
  updated_by = EXCLUDED.updated_by, updated_at = now();

-- name: DeletePlacementMoney :exec
DELETE FROM placement_money WHERE placement_id = @placement_id;

-- Placements starting in a period, with their fee when the user may see it
-- (row-level security leaves the money out otherwise) (reports).
-- name: PlacementFacts :many
SELECT p.id, p.recruiter_id, p.status, p.start_date, m.fee_amount, m.fee_currency
FROM placement p LEFT JOIN placement_money m ON m.tenant_id = p.tenant_id AND m.placement_id = p.id
WHERE p.start_date >= @from_day AND p.start_date <= @to_day
ORDER BY p.start_date
LIMIT 20000;

-- Invoicing: who receives the invoice, the terms and due date, and the
-- emailing on the start date.
-- name: ListPlacementInvoicing :many
SELECT * FROM placement_invoice WHERE placement_id = ANY (@placement_ids::text[]);

-- name: PutPlacementInvoicing :exec
INSERT INTO placement_invoice (tenant_id, placement_id, contact_id, payment_terms_days, due_on, auto_send, updated_by)
VALUES (app_current_tenant(), @placement_id, sqlc.narg(contact_id), @payment_terms_days, sqlc.narg(due_on), @auto_send, app_current_user())
ON CONFLICT (tenant_id, placement_id) DO UPDATE SET
  contact_id = EXCLUDED.contact_id, payment_terms_days = EXCLUDED.payment_terms_days, due_on = EXCLUDED.due_on,
  auto_send = EXCLUDED.auto_send, send_error = NULL, updated_by = EXCLUDED.updated_by, updated_at = now();

-- One sender at a time (the start-date job and "send now").
-- name: LockPlacementInvoicing :one
SELECT * FROM placement_invoice WHERE placement_id = @placement_id FOR UPDATE;

-- name: MarkInvoiceSent :exec
UPDATE placement_invoice SET sent_at = now(), send_error = NULL, due_on = @due_on WHERE placement_id = @placement_id;

-- name: SetInvoiceError :exec
UPDATE placement_invoice SET send_error = @send_error WHERE placement_id = @placement_id;

-- name: MarkPlacementInvoiced :exec
UPDATE placement SET invoice_status = 'invoiced', invoice_number = @invoice_number, invoiced_on = @invoiced_on,
  version = version + 1, updated_at = now()
WHERE id = @id;

-- The agency's invoice template (none: the defaults).
-- name: GetInvoiceSettings :one
SELECT tenant_id, issuer, bank_details, number_prefix, next_number, tax_label, tax_percent::text AS tax_percent, line_text, footer,
       email_subject, email_body, send_hour, version
FROM invoice_settings;

-- name: InsertInvoiceSettings :exec
INSERT INTO invoice_settings (tenant_id, issuer, bank_details, number_prefix, next_number, tax_label, tax_percent, line_text, footer,
                              email_subject, email_body, send_hour, updated_by)
VALUES (app_current_tenant(), @issuer, @bank_details, @number_prefix, @next_number, @tax_label, sqlc.arg(tax_percent)::text::numeric, @line_text,
        @footer, @email_subject, @email_body, @send_hour, app_current_user())
ON CONFLICT (tenant_id) DO NOTHING;

-- name: UpdateInvoiceSettings :execrows
UPDATE invoice_settings
SET issuer = @issuer, bank_details = @bank_details, number_prefix = @number_prefix, next_number = @next_number, tax_label = @tax_label,
    tax_percent = sqlc.arg(tax_percent)::text::numeric, line_text = @line_text, footer = @footer, email_subject = @email_subject,
    email_body = @email_body, send_hour = @send_hour, version = version + 1, updated_by = app_current_user(), updated_at = now()
WHERE version = @expected_version;

-- The next invoice number, taken (the row is locked until the transaction ends).
-- name: TakeInvoiceNumber :one
UPDATE invoice_settings SET next_number = next_number + 1 RETURNING next_number - 1 AS number, number_prefix;

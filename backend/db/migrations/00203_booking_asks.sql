-- scheduling: a booking link can ask for the person's phone number or email
-- (to call them, or to send the meeting), kept with the booking.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE booking_link
  ADD COLUMN ask_phone boolean NOT NULL DEFAULT false,
  ADD COLUMN ask_email boolean NOT NULL DEFAULT false,
  ADD COLUMN given_phone text CHECK (length(given_phone) BETWEEN 1 AND 40),
  ADD COLUMN given_email text CHECK (length(given_email) BETWEEN 3 AND 254);

-- +goose Down
ALTER TABLE booking_link DROP COLUMN IF EXISTS given_email, DROP COLUMN IF EXISTS given_phone,
  DROP COLUMN IF EXISTS ask_email, DROP COLUMN IF EXISTS ask_phone;

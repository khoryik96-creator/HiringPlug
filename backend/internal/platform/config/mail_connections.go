package config

// MailConnections is deployment registration, never a user's mailbox password.
type MailConnections struct {
	KeysSecret            []byte // HP_MAIL_KEYS_SECRET encrypts mailbox credentials and tokens
	MicrosoftClientID     string // HP_MAIL_MICROSOFT_CLIENT_ID
	MicrosoftClientSecret string // HP_MAIL_MICROSOFT_CLIENT_SECRET
	GoogleClientID        string // HP_MAIL_GOOGLE_CLIENT_ID
	GoogleClientSecret    string // HP_MAIL_GOOGLE_CLIENT_SECRET
}

// Dropbox is the deployment's Dropbox app, for uploading claim receipts to
// the company's Dropbox (Google Drive and OneDrive use the mail apps).
type Dropbox struct {
	AppKey    string // HP_DROPBOX_APP_KEY
	AppSecret string // HP_DROPBOX_APP_SECRET
}

package config

// MailConnections is deployment registration, never a user's mailbox password.
type MailConnections struct {
	KeysSecret            []byte // HP_MAIL_KEYS_SECRET encrypts mailbox credentials and tokens
	MicrosoftClientID     string // HP_MAIL_MICROSOFT_CLIENT_ID
	MicrosoftClientSecret string // HP_MAIL_MICROSOFT_CLIENT_SECRET
	GoogleClientID        string // HP_MAIL_GOOGLE_CLIENT_ID
	GoogleClientSecret    string // HP_MAIL_GOOGLE_CLIENT_SECRET
}

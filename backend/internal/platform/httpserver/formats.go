package httpserver

import (
	"net/mail"

	"github.com/getkin/kin-openapi/openapi3"
)

// Formats the contract uses that kin-openapi does not check by itself. They
// run in the contract check, so a bad value is reported against its field
// (e.g. "email") instead of failing later, when the body is decoded, as an
// unreadable "body".
func init() {
	// The same rule the generated types use to decode format: email
	// (openapi_types.Email), so whatever passes here also decodes.
	openapi3.DefineStringFormatValidator("email", openapi3.NewCallbackValidator(func(s string) error {
		if _, err := mail.ParseAddress(s); err != nil {
			return fieldMessage("is not a valid email address")
		}
		return nil
	}))
}

// fieldMessage is a validation message written for people. collectSchemaErrors
// shows it instead of the validator's technical reason.
type fieldMessage string

func (m fieldMessage) Error() string { return string(m) }

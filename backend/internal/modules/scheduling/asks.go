package scheduling

// What a booking link asks for before booking (pure rules): the person's
// phone number, to be called, and their email, to be sent the meeting.

import (
	"net/mail"
	"regexp"
	"slices"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Given is what the person gave when booking.
type Given struct{ Phone, Email string }

var phoneRe = regexp.MustCompile(`^\+?[0-9][0-9 ()\-.]{5,38}$`)

// CheckGiven tidies what was given and says what is missing or wrong: the
// phone number when needPhone, the email when needEmail (either may also be
// given unasked).
func CheckGiven(g Given, needPhone, needEmail bool) (Given, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	g.Phone = strings.Join(strings.Fields(g.Phone), " ")
	g.Email = strings.ToLower(strings.TrimSpace(g.Email))
	switch {
	case g.Phone == "" && needPhone:
		errs.Add("phone", "is needed to call you")
	case g.Phone != "" && !phoneRe.MatchString(g.Phone):
		errs.Add("phone", "is not a phone number, e.g. +60 12 345 6789")
	}
	switch {
	case g.Email == "" && needEmail:
		errs.Add("email", "is needed to send you the meeting")
	case g.Email != "":
		if a, err := mail.ParseAddress(g.Email); err != nil || a.Address != g.Email {
			errs.Add("email", "is not an email address")
		}
	}
	return g, errs
}

func asks(list *[]api.BookingAsk, ask api.BookingAsk) bool {
	return list != nil && slices.Contains(*list, ask)
}

func askList(phone, email bool) []api.BookingAsk {
	out := []api.BookingAsk{}
	if phone {
		out = append(out, api.BookingAskPhone)
	}
	if email {
		out = append(out, api.BookingAskEmail)
	}
	return out
}

func textOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

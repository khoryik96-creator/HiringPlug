package emails

import (
	"context"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func invalidRecipient() error {
	return apperr.Validation(apperr.FieldErrors{{Field: "recipientCandidateId", Message: "choose either a contact or a candidate recipient, alongside selected candidate details"}})
}

// Recipients and selected candidate packages are independent. Greeting fields belong to the recipient.
func (s *Service) recipientValues(ctx context.Context, tx pgx.Tx, p authz.Principal, values map[string]string, contactID, recipientID *string, loc *time.Location) error {
	if contactID == nil && recipientID == nil {
		return nil
	}
	recipient, err := s.values(ctx, tx, p, recipientID, nil, nil, contactID, loc)
	if err != nil {
		return err
	}
	for key, value := range recipient {
		values[key] = value
	}
	return nil
}

// salaryOnlyTheirOwn: an email to a candidate never shows another
// candidate's salary. A salary field inside another candidate's details block
// (data-candidate-id) is refused; one filled into the message itself is the
// recipient's own (their values fill the message's fields) and may go.
func salaryOnlyTheirOwn(recipientID *string, bodyHTML string, showsSalary bool) error {
	if recipientID == nil || !showsSalary {
		return nil
	}
	for _, owner := range salaryOwners(bodyHTML) {
		if owner != "" && owner != *recipientID {
			return apperr.Validation(apperr.FieldErrors{{Field: "bodyHtml", Message: "another candidate's salary cannot be sent to a candidate; untick the salary details"}})
		}
	}
	return nil
}

// salaryOwners: for each salary field in the message, the candidate whose
// details block holds it ("" when it is in the message itself).
func salaryOwners(bodyHTML string) []string {
	nodes, err := html.ParseFragment(strings.NewReader(bodyHTML), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return []string{"unreadable"} // refuse what cannot be checked
	}
	owners := []string{}
	var walk func(n *html.Node, owner string)
	walk = func(n *html.Node, owner string) {
		if n.Type == html.ElementNode {
			if id := attr(n, "data-candidate-id"); id != "" {
				owner = id
			}
			if f, ok := fieldByKey(attr(n, "data-field")); ok && f.Salary {
				owners = append(owners, owner)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, owner)
		}
	}
	for _, n := range nodes {
		walk(n, "")
	}
	return owners
}

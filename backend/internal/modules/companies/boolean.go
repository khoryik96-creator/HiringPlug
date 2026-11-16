package companies

import (
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/boolsearch"
)

// booleanQuery reads a boolean search into the query for to_tsquery (nil:
// none asked for); a mistyped one says what is wrong with it.
func booleanQuery(boolean *string) (*string, error) {
	if boolean == nil {
		return nil, nil
	}
	q, err := boolsearch.Parse(*boolean)
	if err != nil {
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "boolean", Message: err.Error()}})
	}
	ts := q.TSQuery()
	return &ts, nil
}

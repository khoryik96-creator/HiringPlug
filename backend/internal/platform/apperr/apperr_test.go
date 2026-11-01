package apperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// Every error code in the contract must map to a real HTTP status.
func TestEveryErrorCodeHasAStatus(t *testing.T) {
	spec, err := api.Spec()
	assert.NoError(t, err)
	for _, v := range spec.Components.Schemas["ErrorCode"].Value.Enum {
		code := api.ErrorCode(v.(string))
		s := Status(code)
		assert.True(t, s >= 400 && s <= 599, "%s has status %d", code, s)
	}
	assert.Equal(t, http.StatusConflict, Status(api.ErrorCodeCANDIDATEDUPLICATEEMAIL))
}

func TestErrorsAreFoundThroughWrapping(t *testing.T) {
	err := fmt.Errorf("saving: %w", NotFound("candidate"))
	e, ok := As(err)
	assert.True(t, ok)
	assert.Equal(t, api.ErrorCodeNOTFOUND, e.Code)

	_, ok = As(errors.New("plain"))
	assert.False(t, ok)
}

func TestNoFieldErrorsMeansNoError(t *testing.T) {
	var f FieldErrors
	assert.NoError(t, f.Err())
	f.Add("email", "is invalid")
	assert.Error(t, f.Err())
}

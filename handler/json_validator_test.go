package handler

import (
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/stretchr/testify/require"
)

func TestInvalid(t *testing.T) {
	var errBody = []byte(`{"display_names":"Barth"}`)
	var attr model.UserAttributes
	err := ValidateBody(errBody, &attr)
	require.Error(t, err)
}

func TestValid(t *testing.T) {
	var errBody = []byte(`{"display_name":"Barth"}`)
	var attr model.UserAttributes
	err := ValidateBody(errBody, &attr)
	require.NoError(t, err)
}

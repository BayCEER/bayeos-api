package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetNodePrivsByUser(t *testing.T) {
	jt, err := SignIn("root", "bayeos")
	require.NoError(t, err)

	user, err := CurrentUser(jt)
	require.NoError(t, err)
	require.NotNil(t, user)

	req, err := NewJWTRequest("GET", fmt.Sprintf("/api/users/%d/privileges/grantee", user.ID), nil, jt)
	require.NoError(t, err)
	require.NotNil(t, req)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req, err = NewJWTRequest("GET", fmt.Sprintf("/api/users/%d/privileges/grantor", user.ID), nil, jt)
	require.NoError(t, err)
	require.NotNil(t, req)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

}

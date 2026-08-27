package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

// SignUp + SignIn + RefreshIDToken + SignOut
func TestSignUpOut(t *testing.T) {

	// SignUp
	suc := model.SignUpCredentials{
		SignInName:   "dummy",
		DisplayName:  "Dummy User",
		EmailAddress: "dummy@dummy.com",
		Password:     "dummyPassword"}
	req, err := NewRequest("POST", "/api/auth/signup", suc)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// SignIn
	token, err := SignIn("dummy", "dummyPassword")
	require.NoError(t, err)

	// Refresh
	req, err = NewJWTRequest("PUT", "/api/auth/refresh", nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var re interface{}
	err = json.Unmarshal(w.Body.Bytes(), &re)
	require.NoError(t, err)

	rt := re.(map[string]interface{})
	require.NotNil(t, rt)
	var rtoken = rt["token"].(string)

	// Change Password
	req, err = NewJWTRequest("PUT", "/api/auth/password", model.SignInCredentials{Name: "dummy", Password: "newpassword"}, rtoken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// SignIn Fails
	_, err = SignIn("dummy", "dummyPassword")
	require.Error(t, err)

	// SignIn Succeeds
	token, err = SignIn("dummy", "newpassword")
	require.NoError(t, err)

	// SignOut
	cre := model.SignInCredentials{Name: "dummy", Password: "newpassword"}
	req, err = NewJWTRequest("POST", "/api/auth/signout", cre, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// SignIn Fails
	_, err = SignIn("dummy", "newpassword")
	require.Error(t, err)

}

func TestWrongPassword(t *testing.T) {
	_, err := SignIn("root", "wrongpassword")
	require.NotNil(t, err)
}

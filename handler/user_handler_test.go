package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/stretchr/testify/require"
)

func TestUserCRUD(t *testing.T) {

	// SignIn as root
	jt, err := SignIn("root", "bayeos")
	require.NoError(t, err)

	// GetUsers
	req, err := NewJWTRequest("GET", "/api/users", nil, jt)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var users []model.User
	err = json.Unmarshal(w.Body.Bytes(), &users)
	require.NoError(t, err)

	userCount := len(users)

	// SignUp New User
	signInName := "lisa"
	password := "secretpassword"
	req, err = NewRequest("POST", "/api/auth/signup", model.SignUpCredentials{SignInName: signInName,
		DisplayName: "Lisa Simpson", EmailAddress: "lisa@simpsons.com", Password: password})
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// SignIn New User
	token, err := SignIn(signInName, password)
	require.NoError(t, err)

	// GetUsers -> assert new user is in list
	req, err = NewJWTRequest("GET", "/api/users", nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &users)
	require.NoError(t, err)
	require.Equal(t, userCount+1, len(users))

	var id int32
	for _, v := range users {
		if v.SignInName == signInName {
			id = v.ID
		}

	}
	require.NotNil(t, id)

	// Patch user's attributes
	var patch = model.UserAttributes{DisplayName: "barti", SignInName: "barti", EmailAddress: "barti@simpsons.com"}
	req, err = NewJWTRequest("PATCH", fmt.Sprintf("/api/users/%d", id), patch, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// GetUser by id
	req, err = NewJWTRequest("GET", fmt.Sprintf("/api/users/%d", id), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var ret = model.User{}
	err = json.Unmarshal(w.Body.Bytes(), &ret)

	require.NoError(t, err)
	require.NotNil(t, ret)
	require.NotNil(t, ret.ID)
	require.Equal(t, "barti", ret.SignInName)
	require.Equal(t, "barti@simpsons.com", ret.EmailAddress)
	require.Equal(t, "barti", ret.DisplayName)

	// Delete user as super user
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/api/users/%d", id), nil, jt)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// GetUser by id fails
	req, err = NewJWTRequest("GET", fmt.Sprintf("/api/users/%d", id), nil, jt)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

}

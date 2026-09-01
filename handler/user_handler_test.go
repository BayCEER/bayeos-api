package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
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

	// Non super user cannot fetch another user's profile
	req, err = NewJWTRequest("GET", "/api/users/1", nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

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

func TestCreateUser(t *testing.T) {

	// SignIn as super user
	jt, err := SignIn("root", "bayeos")
	require.NoError(t, err)

	// Create user via /api/users
	suc := model.SignUpCredentials{SignInName: "seymour", DisplayName: "Seymour Skinner",
		EmailAddress: "seymour@simpsons.com", Password: "secretpassword"}
	req, err := NewJWTRequest("POST", "/api/users", suc, jt)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var user model.User
	err = json.Unmarshal(w.Body.Bytes(), &user)
	require.NoError(t, err)
	require.NotEqual(t, int32(0), user.ID)
	require.Equal(t, "seymour", user.SignInName)
	require.Equal(t, "Seymour Skinner", user.DisplayName)
	require.Equal(t, "seymour@simpsons.com", user.EmailAddress)
	require.False(t, user.SuperUser)

	// Verify persisted user is readable by id
	req, err = NewJWTRequest("GET", fmt.Sprintf("/api/users/%d", user.ID), nil, jt)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var ret model.User
	err = json.Unmarshal(w.Body.Bytes(), &ret)
	require.NoError(t, err)
	require.Equal(t, user.ID, ret.ID)

	// Non super user is not allowed to create users
	nonSuperToken, err := SignUp("edna", "secretpassword")
	require.NoError(t, err)
	req, err = NewJWTRequest("POST", "/api/users", suc, nonSuperToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	// Too short password fails user creation
	shortPw := model.SignUpCredentials{SignInName: "waylon", DisplayName: "Waylon Smithers",
		EmailAddress: "waylon@simpsons.com", Password: "123"}
	req, err = NewJWTRequest("POST", "/api/users", shortPw, jt)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)

}

func TestUserImageCRUD(t *testing.T) {

	// Image owner
	token, err := SignUp("apu", "secretpassword")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	// Get image before it exists -> 404
	req, err := NewJWTRequest("GET", fmt.Sprintf("/api/users/%d/image", user.ID), nil, token)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	// Create image from raw body
	jpegData := bytes.Repeat([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, 64)
	req, err = NewJWTReaderRequest("POST", fmt.Sprintf("/api/users/%d/image", user.ID), bytes.NewReader(jpegData), token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Get image -> 200 with exactly the uploaded bytes and JPEG content type
	req, err = NewJWTRequest("GET", fmt.Sprintf("/api/users/%d/image", user.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	require.Equal(t, jpegData, w.Body.Bytes())

	// Upload unsupported format -> 415
	req, err = NewJWTReaderRequest("POST", fmt.Sprintf("/api/users/%d/image", user.ID), bytes.NewReader([]byte("this is not an image")), token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnsupportedMediaType, w.Code)

	// Upload image exceeding the maximum size -> 413
	oversized := bytes.Repeat([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, service.MaxUserImageBytes/6+2)
	req, err = NewJWTReaderRequest("POST", fmt.Sprintf("/api/users/%d/image", user.ID), bytes.NewReader(oversized), token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

	// Re-upload as PNG replaces the stored image
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}
	req, err = NewJWTReaderRequest("POST", fmt.Sprintf("/api/users/%d/image", user.ID), bytes.NewReader(pngData), token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Get image -> PNG content type and exactly the re-uploaded bytes
	req, err = NewJWTRequest("GET", fmt.Sprintf("/api/users/%d/image", user.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))
	require.Equal(t, pngData, w.Body.Bytes())

	// Another non-super user cannot read, create or delete the image of others
	otherToken, err := SignUp("moe", "secretpassword")
	require.NoError(t, err)
	req, err = NewJWTRequest("GET", fmt.Sprintf("/api/users/%d/image", user.ID), nil, otherToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	req, err = NewJWTReaderRequest("POST", fmt.Sprintf("/api/users/%d/image", user.ID), bytes.NewReader(jpegData), otherToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/api/users/%d/image", user.ID), nil, otherToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	// Delete image -> 200
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/api/users/%d/image", user.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Delete image again -> 404 (no row affected)
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/api/users/%d/image", user.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	// Get image after deletion -> 404
	req, err = NewJWTRequest("GET", fmt.Sprintf("/api/users/%d/image", user.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

}

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateGetDeleteNodeToken(t *testing.T) {

	// SignIn
	jt, err := SignIn("root", "bayeos")
	require.NoError(t, err)

	user, err := CurrentUser(jt)
	require.NoError(t, err)
	require.NotNil(t, user)

	// POST
	uu, err := uuid.NewRandom()
	require.NoError(t, err)
	nodetoken := model.NodeToken{NodeID: user.HomeNodeID, Token: uu}
	req, err := NewJWTRequest("POST", fmt.Sprintf("/api/nodes/%d/tokens", user.HomeNodeID), nodetoken, jt)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var nt = model.NodeToken{}
	err = json.Unmarshal(w.Body.Bytes(), &nt)
	require.Nil(t, err)

	// GET
	req, err = NewJWTRequest("GET", fmt.Sprintf("/api/nodes/%d/tokens/%d", user.HomeNodeID, nt.ID), nil, jt)
	require.NoError(t, err)
	require.NotNil(t, req)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var ret = model.NodeToken{}
	err = json.Unmarshal(w.Body.Bytes(), &ret)
	require.NoError(t, err)
	require.NotNil(t, ret)
	require.NotNil(t, ret.ID)
	require.Equal(t, nt.Token, ret.Token)

	// DELETE
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/api/nodes/%d/tokens/%d", user.HomeNodeID, ret.ID), nil, jt)
	require.NoError(t, err)
	require.NotNil(t, req)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

}

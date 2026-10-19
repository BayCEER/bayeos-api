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

// newNodeTokenRequest is NewRequest with a Node-Token header instead of a JWT.
func newNodeTokenRequest(method, url string, body any, nodeToken string) (*http.Request, error) {
	return newRequest(method, url, body, map[string]string{"Node-Token": nodeToken})
}

func createChildNode(t *testing.T, jwt string, parentID int32, name string) model.Node {
	t.Helper()
	req, err := NewJWTRequest("POST", "/bayeos-api/nodes", model.Node{DisplayName: name, ParentID: parentID}, jwt)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var n model.Node
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &n))
	return n
}

func createNodeToken(t *testing.T, jwt string, nodeID int32, body any) model.NodeToken {
	t.Helper()
	req, err := NewJWTRequest("POST", fmt.Sprintf("/bayeos-api/nodes/%d/tokens", nodeID), body, jwt)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var nt model.NodeToken
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &nt))
	return nt
}

// TestNodeTokenAuthenticatesNodeRoutes exercises the whole node-token path
// through the real router: a token acts on its subtree, is bound by its
// permission bits, cannot reach token management, and stops working when
// deactivated.
func TestNodeTokenAuthenticatesNodeRoutes(t *testing.T) {
	jwt, err := SignUp("wiggum", "springfield1")
	require.NoError(t, err)
	user, err := CurrentUser(jwt)
	require.NoError(t, err)

	child := createChildNode(t, jwt, user.HomeNodeID, "Server")

	// A default token carries full permission.
	full := createNodeToken(t, jwt, child.ID, nil)
	require.EqualValues(t, model.FULL_PERMISSION, full.Permission)

	// It can read the node it was minted on.
	req, err := newNodeTokenRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", child.ID), nil, full.Token.String())
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// It cannot read the parent, which is outside its subtree.
	req, err = newNodeTokenRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", user.HomeNodeID), nil, full.Token.String())
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	// Token management stays JWT-only.
	req, err = newNodeTokenRequest("POST", fmt.Sprintf("/bayeos-api/nodes/%d/tokens", child.ID), nil, full.Token.String())
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	// A read-only token may read but not write.
	readOnly := createNodeToken(t, jwt, child.ID, map[string]any{"permission": model.READ_PERMISSION})
	require.EqualValues(t, model.READ_PERMISSION, readOnly.Permission)

	req, err = newNodeTokenRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", child.ID), nil, readOnly.Token.String())
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req, err = newNodeTokenRequest("PATCH", fmt.Sprintf("/bayeos-api/nodes/%d", child.ID), map[string]any{"display_name": "nope"}, readOnly.Token.String())
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	// Deactivating a token makes it unusable.
	req, err = NewJWTRequest("PATCH", fmt.Sprintf("/bayeos-api/nodes/%d/tokens/%d", child.ID, full.ID), map[string]any{"active": false}, jwt)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req, err = newNodeTokenRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", child.ID), nil, full.Token.String())
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

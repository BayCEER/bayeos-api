package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/stretchr/testify/require"
)

func TestNodeCRUD(t *testing.T) {

	token, err := SignUp("homer", "simpson")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	// CREATE
	node := model.Node{DisplayName: "Private Folder", ParentID: user.HomeNodeID}
	req, err := NewJWTRequest("POST", "/bayeos-api/nodes", node, token)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var newNode model.Node
	err = json.Unmarshal(w.Body.Bytes(), &newNode)
	require.NoError(t, err)
	require.NotNil(t, newNode.ID)
	require.Equal(t, node.ParentID, newNode.ParentID)
	require.Equal(t, node.DisplayName, newNode.DisplayName)

	// PATCH
	var patch = []byte(`{"display_name":"Springfield"}`)
	req, err = NewJWTReaderRequest("PATCH", fmt.Sprintf("/bayeos-api/nodes/%d", newNode.ID), bytes.NewBuffer(patch), token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// GET
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", newNode.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &node)
	require.NoError(t, err)
	require.Equal(t, newNode.ID, node.ID)
	require.Equal(t, "Springfield", node.DisplayName)

	// PATCH with read-only node_path is rejected and leaves the path unchanged
	forgedPath := node.NodePath
	req, err = NewJWTReaderRequest("PATCH", fmt.Sprintf("/bayeos-api/nodes/%d", newNode.ID), bytes.NewBuffer([]byte(`{"node_path":"/forged"}`)), token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", newNode.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &node)
	require.NoError(t, err)
	require.Equal(t, forgedPath, node.NodePath)

	// CREATE node token
	req, err = NewJWTRequest("POST", fmt.Sprintf("/bayeos-api/nodes/%d/tokens", newNode.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// GET node tokens
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/tokens", newNode.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var nodetokens []model.NodeToken
	err = json.Unmarshal(w.Body.Bytes(), &nodetokens)
	require.NoError(t, err)
	require.Equal(t, 1, len(nodetokens))

	// Get node token by id
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/tokens/%d", newNode.ID, nodetokens[0].ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var nodetoken model.NodeToken
	err = json.Unmarshal(w.Body.Bytes(), &nodetoken)
	require.NoError(t, err)
	require.Equal(t, nodetokens[0].ID, nodetoken.ID)
	require.Equal(t, nodetokens[0].NodeID, nodetoken.NodeID)
	require.Equal(t, nodetokens[0].Token, nodetoken.Token)

	// DELETE node token
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/nodes/%d/tokens/%d", newNode.ID, nodetokens[0].ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// DELETE node token again (should fail)
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/nodes/%d/tokens/%d", newNode.ID, nodetokens[0].ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	// DELETE node
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/nodes/%d", newNode.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// DELETE node again (should fail): the permission check runs before the
	// handler and is keyed on the node's own id, so a missing node is rejected as Forbidden.
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/nodes/%d", newNode.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

}

func TestNodeMetadataNullDeletesKey(t *testing.T) {
	token, err := SignUp("barney", "slurms")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	// CREATE with two metadata keys
	node := model.Node{
		DisplayName: "Moe's Bar",
		ParentID:    user.HomeNodeID,
		Metadata:    model.NewMetadata([]byte(`{"unit":"V","own":"x"}`)),
	}
	req, err := NewJWTRequest("POST", "/bayeos-api/nodes", node, token)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var newNode model.Node
	err = json.Unmarshal(w.Body.Bytes(), &newNode)
	require.NoError(t, err)

	// PATCH one key to null and update the other in the same write
	patch := []byte(`{"metadata":{"own":null,"unit":"°C"}}`)
	req, err = NewJWTReaderRequest("PATCH", fmt.Sprintf("/bayeos-api/nodes/%d", newNode.ID), bytes.NewBuffer(patch), token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// GET: the nulled key is gone, the other key merged
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", newNode.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got model.Node
	err = json.Unmarshal(w.Body.Bytes(), &got)
	require.NoError(t, err)

	md := got.Metadata.Map()
	_, ownPresent := md["own"]
	require.False(t, ownPresent, "a key sent as null must be deleted, not stored as null")
	require.Equal(t, "°C", md["unit"])

	// DELETE the node again
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/nodes/%d", newNode.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestGetRootNode(t *testing.T) {
	// New user
	token, err := SignUp("marge", "123456789")
	require.NoError(t, err)

	// Get root node
	req, err := NewJWTRequest("GET", "/bayeos-api/nodes", nil, token)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var nodes []model.Node
	err = json.Unmarshal(w.Body.Bytes(), &nodes)
	require.NoError(t, err)
	require.Equal(t, 1, len(nodes))
	require.Equal(t, int32(1), nodes[0].ID)

	// Try to delete root
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/nodes/%d", nodes[0].ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestSearchNodes(t *testing.T) {
	token, err := SignUp("antuser", "antpass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	// CREATE 3 Folder nodes and 1 Timeseries node with a common name prefix
	for i := 0; i < 3; i++ {
		node := model.Node{DisplayName: fmt.Sprintf("AntTarget-%d", i), Kind: "Folder", ParentID: user.HomeNodeID}
		req, err := NewJWTRequest("POST", "/bayeos-api/nodes", node, token)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)
	}

	tsNode := model.Node{DisplayName: "AntTarget-99", Kind: "Timeseries", ParentID: user.HomeNodeID}
	req, err := NewJWTRequest("POST", "/bayeos-api/nodes", tsNode, token)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Search by Ant pattern across all kinds (kind omitted -> all kinds matched)
	req, err = NewJWTRequest("GET", "/bayeos-api/nodes/search?pattern=**/AntTarget*", nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var searchResult model.NodeSearch
	err = json.Unmarshal(w.Body.Bytes(), &searchResult)
	require.NoError(t, err)
	require.Equal(t, int32(4), searchResult.Total)
	require.Equal(t, 4, len(searchResult.Hits))

	// Filter by kind=Folder -> only the 3 Folder nodes
	req, err = NewJWTRequest("GET", "/bayeos-api/nodes/search?pattern=**/AntTarget*&kind=Folder", nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &searchResult)
	require.NoError(t, err)
	require.Equal(t, int32(3), searchResult.Total)
	require.Equal(t, 3, len(searchResult.Hits))

	// Filter by kind=Timeseries -> only 1 node
	req, err = NewJWTRequest("GET", "/bayeos-api/nodes/search?pattern=**/AntTarget*&kind=Timeseries", nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &searchResult)
	require.NoError(t, err)
	require.Equal(t, int32(1), searchResult.Total)
	require.Equal(t, 1, len(searchResult.Hits))

	// A leading "~" stands for the caller's home node
	req, err = NewJWTRequest("GET", "/bayeos-api/nodes/search?pattern=~/**", nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &searchResult)
	require.NoError(t, err)
	require.Equal(t, int32(4), searchResult.Total)
	require.Equal(t, 4, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		require.True(t, strings.HasPrefix(hit.NodePath, "/Users/antuser/"),
			"hit %q is not below the caller's home", hit.NodePath)
	}

	// "~" on its own is the home node itself
	req, err = NewJWTRequest("GET", "/bayeos-api/nodes/search?pattern=~", nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &searchResult)
	require.NoError(t, err)
	require.Equal(t, int32(1), searchResult.Total)
	require.Equal(t, user.HomeNodeID, searchResult.Hits[0].ID)

	// The home node of one user is not the home node of another, so "~" must not
	// widen into a match on somebody else's tree.
	otherToken, err := SignUp("otherant", "antpass")
	require.NoError(t, err)
	req, err = NewJWTRequest("GET", "/bayeos-api/nodes/search?pattern=~/**", nil, otherToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &searchResult)
	require.NoError(t, err)
	require.Equal(t, int32(0), searchResult.Total)

	// Missing pattern is a bad request
	req, err = NewJWTRequest("GET", "/bayeos-api/nodes/search", nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestParentChildRelationship(t *testing.T) {
	token, err := SignUp("krusty", "secretpassword")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	// CREATE child node
	node := model.Node{DisplayName: "Krusty Files", ParentID: user.HomeNodeID}
	req, err := NewJWTRequest("POST", "/bayeos-api/nodes", node, token)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var child model.Node
	err = json.Unmarshal(w.Body.Bytes(), &child)
	require.NoError(t, err)
	require.NotEqual(t, int32(0), child.ID)
	require.Equal(t, user.HomeNodeID, child.ParentID)

	// GET children of home node -> child is listed
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/children", user.HomeNodeID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var children []model.Node
	err = json.Unmarshal(w.Body.Bytes(), &children)
	require.NoError(t, err)
	require.Equal(t, 1, len(children))
	require.Equal(t, child.ID, children[0].ID)

	// GET parents of child node -> home node and the child itself are returned
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/parents", child.ID), nil, token)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var parents []model.Node
	err = json.Unmarshal(w.Body.Bytes(), &parents)
	require.NoError(t, err)

	parentIDs := make([]int32, len(parents))
	for i, p := range parents {
		parentIDs[i] = p.ID
	}
	require.Contains(t, parentIDs, child.ID)
	require.Contains(t, parentIDs, user.HomeNodeID)

	// User without permission cannot read children of foreign node
	otherToken, err := SignUp("sideshowbob", "secretpassword")
	require.NoError(t, err)
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/children", user.HomeNodeID), nil, otherToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

}

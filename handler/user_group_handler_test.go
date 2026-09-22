package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

func userIDs(users []model.User) []int32 {
	ids := make([]int32, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	return ids
}

func TestUserGroupCRUD(t *testing.T) {
	rootToken, err := SignIn("root", "bayeos")
	require.NoError(t, err)

	// CREATE
	req, err := NewJWTRequest("POST", "/bayeos-api/usergroups", model.UserGroupCreate{DisplayName: "TestGroup"}, rootToken)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created model.UserGroup
	err = json.Unmarshal(w.Body.Bytes(), &created)
	require.NoError(t, err)
	require.NotEqual(t, int32(0), created.ID)
	require.Equal(t, "TestGroup", created.DisplayName)

	// GET by id
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/usergroups/%d", created.ID), nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got model.UserGroup
	err = json.Unmarshal(w.Body.Bytes(), &got)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "TestGroup", got.DisplayName)
	require.Empty(t, got.MemberIDs)

	// UPDATE display_name
	req, err = NewJWTReaderRequest("PATCH", fmt.Sprintf("/bayeos-api/usergroups/%d", created.ID),
		bytes.NewBuffer([]byte(`{"display_name":"RenamedGroup"}`)), rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Verify updated name
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/usergroups/%d", created.ID), nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &got)
	require.NoError(t, err)
	require.Equal(t, "RenamedGroup", got.DisplayName)

	// LIST
	req, err = NewJWTRequest("GET", "/bayeos-api/usergroups", nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var page model.Page[model.UserGroup]
	err = json.Unmarshal(w.Body.Bytes(), &page)
	require.NoError(t, err)
	require.Equal(t, len(page.Hits), page.Total)
	require.Contains(t, page.Hits, got)

	// MEMBERS: create two users, add them to the group
	u1Token, err := SignUp("ned", "secretpassword")
	require.NoError(t, err)
	u1, err := CurrentUser(u1Token)
	require.NoError(t, err)
	u2Token, err := SignUp("flanders", "secretpassword")
	require.NoError(t, err)
	u2, err := CurrentUser(u2Token)
	require.NoError(t, err)

	req, err = NewJWTRequest("POST", fmt.Sprintf("/bayeos-api/usergroups/%d/members", created.ID),
		model.UserGroupMembersRequest{UserIDs: []int32{u1.ID, u2.ID}}, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Get members
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/usergroups/%d/members", created.ID), nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var members []model.User
	err = json.Unmarshal(w.Body.Bytes(), &members)
	require.NoError(t, err)
	memberIDs := userIDs(members)
	require.Contains(t, memberIDs, u1.ID)
	require.Contains(t, memberIDs, u2.ID)

	// Group response includes member ids
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/usergroups/%d", created.ID), nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &got)
	require.NoError(t, err)
	require.ElementsMatch(t, []int32{u1.ID, u2.ID}, got.MemberIDs)

	// Remove member
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/usergroups/%d/members/%d", created.ID, u1.ID), nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Members list no longer contains the removed user
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/usergroups/%d/members", created.ID), nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var membersAfter []model.User
	err = json.Unmarshal(w.Body.Bytes(), &membersAfter)
	require.NoError(t, err)
	memberIDsAfter := userIDs(membersAfter)
	require.NotContains(t, memberIDsAfter, u1.ID)
	require.Contains(t, memberIDsAfter, u2.ID)

	// DELETE group
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/usergroups/%d", created.ID), nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// GET deleted group -> not found
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/usergroups/%d", created.ID), nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserGroupForbiddenForNonSuper(t *testing.T) {
	userToken, err := SignUp("nonadmin", "secretpassword")
	require.NoError(t, err)

	req, err := NewJWTRequest("GET", "/bayeos-api/usergroups", nil, userToken)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestUserGroupPrivilegeGrant(t *testing.T) {
	rootToken, err := SignIn("root", "bayeos")
	require.NoError(t, err)

	// A group member
	memberToken, err := SignUp("otto", "secretpassword")
	require.NoError(t, err)
	member, err := CurrentUser(memberToken)
	require.NoError(t, err)

	// A node owner whose private home node the member has no access to
	ownerToken, err := SignUp("skinner", "secretpassword")
	require.NoError(t, err)
	owner, err := CurrentUser(ownerToken)
	require.NoError(t, err)

	// Create group and add member
	req, err := NewJWTRequest("POST", "/bayeos-api/usergroups", model.UserGroupCreate{DisplayName: "Viewers"}, rootToken)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var group model.UserGroup
	err = json.Unmarshal(w.Body.Bytes(), &group)
	require.NoError(t, err)

	req, err = NewJWTRequest("POST", fmt.Sprintf("/bayeos-api/usergroups/%d/members", group.ID),
		model.UserGroupMembersRequest{UserIDs: []int32{member.ID}}, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Member cannot read owner's home node yet
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", owner.HomeNodeID), nil, memberToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	// Root grants READ to the group on owner's home node
	req, err = NewJWTRequest("POST", fmt.Sprintf("/bayeos-api/nodes/%d/privileges", owner.HomeNodeID),
		model.NodePrivilege{GranteeGroupID: null.IntFrom(int64(group.ID)), Permission: model.READ_PERMISSION}, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Group privileges listing
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/usergroups/%d/privileges", group.ID), nil, rootToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var privs []model.NodePrivilege
	err = json.Unmarshal(w.Body.Bytes(), &privs)
	require.NoError(t, err)
	require.Len(t, privs, 1)
	require.Equal(t, owner.HomeNodeID, privs[0].NodeID)
	require.Equal(t, group.ID, int32(privs[0].GranteeGroupID.ValueOrZero()))
	require.Equal(t, model.READ_PERMISSION, privs[0].Permission)

	// Member can now read owner's home node
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", owner.HomeNodeID), nil, memberToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Member still cannot write (only READ was granted)
	req, err = NewJWTReaderRequest("PATCH", fmt.Sprintf("/bayeos-api/nodes/%d", owner.HomeNodeID),
		bytes.NewBuffer([]byte(`{"display_name":"hacked"}`)), memberToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

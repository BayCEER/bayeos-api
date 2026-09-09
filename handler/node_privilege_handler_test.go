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
)

func TestGetNodePrivsByUser(t *testing.T) {
	jt, err := SignIn("root", "bayeos")
	require.NoError(t, err)

	user, err := CurrentUser(jt)
	require.NoError(t, err)
	require.NotNil(t, user)

	req, err := NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/users/%d/privileges", user.ID), nil, jt)
	require.NoError(t, err)
	require.NotNil(t, req)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

}

func TestNodePrivilegeCRUD(t *testing.T) {

	// Grantee and grantor
	grantorToken, err := SignUp("carl", "secretpassword")
	require.NoError(t, err)
	grantor, err := CurrentUser(grantorToken)
	require.NoError(t, err)

	granteeToken, err := SignUp("lenny", "secretpassword")
	require.NoError(t, err)
	grantee, err := CurrentUser(granteeToken)
	require.NoError(t, err)

	// CREATE privilege on grantor's home node
	priv := model.NodePrivilege{GranteeID: grantee.ID, GrantorID: grantor.ID, Permission: model.READ_PERMISSION}
	req, err := NewJWTRequest("POST", fmt.Sprintf("/bayeos-api/nodes/%d/privileges", grantor.HomeNodeID), priv, grantorToken)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created model.NodePrivilege
	err = json.Unmarshal(w.Body.Bytes(), &created)
	require.NoError(t, err)
	require.NotEqual(t, int32(0), created.ID)
	require.Equal(t, grantor.HomeNodeID, created.NodeID)
	require.Equal(t, grantee.ID, created.GranteeID)
	require.Equal(t, model.READ_PERMISSION, created.Permission)

	// GET all privileges of the node -> created privilege is listed
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/privileges", grantor.HomeNodeID), nil, grantorToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var privs []model.NodePrivilege
	err = json.Unmarshal(w.Body.Bytes(), &privs)
	require.NoError(t, err)
	require.Contains(t, privs, created)

	// GET single privilege by id
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/privileges/%d", grantor.HomeNodeID, created.ID), nil, grantorToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got model.NodePrivilege
	err = json.Unmarshal(w.Body.Bytes(), &got)
	require.NoError(t, err)
	require.Equal(t, created, got)

	// UPDATE permission (read+write)
	var patch = fmt.Appendf(nil, `{"permission": %d}`, model.READ_PERMISSION+model.WRITE_PERMISSION)
	req, err = NewJWTReaderRequest("PATCH", fmt.Sprintf("/bayeos-api/nodes/%d/privileges/%d", grantor.HomeNodeID, created.ID), bytes.NewBuffer(patch), grantorToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Verify updated permission
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/privileges/%d", grantor.HomeNodeID, created.ID), nil, grantorToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &got)
	require.NoError(t, err)
	require.Equal(t, model.READ_PERMISSION+model.WRITE_PERMISSION, got.Permission)

	// DELETE privilege
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/nodes/%d/privileges/%d", grantor.HomeNodeID, created.ID), nil, grantorToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// DELETE again -> not found
	req, err = NewJWTRequest("DELETE", fmt.Sprintf("/bayeos-api/nodes/%d/privileges/%d", grantor.HomeNodeID, created.ID), nil, grantorToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	// GET deleted privilege -> not found
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/privileges/%d", grantor.HomeNodeID, created.ID), nil, grantorToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	// Foreign user without granted privileges is forbidden
	req, err = NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d/privileges", grantor.HomeNodeID), nil, granteeToken)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

}

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeIDToken implements just the ValidateToken half of service.IDTokenService.
type fakeIDToken struct {
	service.IDTokenService
	user model.User
	err  error
}

func (f fakeIDToken) ValidateToken(context.Context, string) (model.User, error) {
	return f.user, f.err
}

// fakeNodeToken implements just the authentication lookup.
type fakeNodeToken struct {
	service.NodeTokenService
	token model.NodeToken
	err   error
}

func (f fakeNodeToken) FindNodeTokenByValue(context.Context, uuid.UUID) (model.NodeToken, error) {
	return f.token, f.err
}

// accessStub is a model.NodeAccess with fixed bits.
type accessStub struct{ perm uint8 }

func (a accessStub) Permission(context.Context, int32) (uint8, error) { return a.perm, nil }

// fakeFactory records how it was asked to build the access subject.
type fakeFactory struct {
	userCalls  int
	tokenCalls int
	lastUserID int32
	lastNodeID int32
	lastPerm   uint8
}

func (f *fakeFactory) ForUser(userID int32) model.NodeAccess {
	f.userCalls++
	f.lastUserID = userID
	return accessStub{perm: model.FULL_PERMISSION}
}

func (f *fakeFactory) ForToken(nodeID int32, permission uint8) model.NodeAccess {
	f.tokenCalls++
	f.lastNodeID = nodeID
	f.lastPerm = permission
	return accessStub{perm: permission}
}

// serve runs middleware through the error-handling Middleware and returns the
// response, the context seen by the terminal handler (nil when the chain was
// aborted before it) and the recorder.
func serve(t *testing.T, method, target string, headers map[string]string, chain ...gin.HandlerFunc) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var seen *gin.Context
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.Use(Middleware())
	handlers := append([]gin.HandlerFunc{}, chain...)
	handlers = append(handlers, func(c *gin.Context) { seen = c })
	engine.Handle(method, target, handlers...)

	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	engine.ServeHTTP(w, req)
	return seen, w
}

func TestAuthUserOrNodeTokenWithUser(t *testing.T) {
	f := &fakeFactory{}
	mw := AuthUserOrNodeToken(
		fakeIDToken{user: model.User{ID: 42}},
		fakeNodeToken{err: errors.New("must not be called")},
		f,
	)

	c, w := serve(t, http.MethodGet, "/nodes/1", map[string]string{"Authorization": "Bearer good"}, mw)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, c)
	u, ok := AuthUserFromContext(c)
	require.True(t, ok)
	require.EqualValues(t, 42, u.ID)
	_, ok = NodeAccessFromContext(c)
	require.True(t, ok)
	require.Equal(t, 1, f.userCalls)
	require.Equal(t, int32(42), f.lastUserID)
	require.Zero(t, f.tokenCalls)
}

func TestAuthUserOrNodeTokenWithNodeToken(t *testing.T) {
	id := uuid.New()
	f := &fakeFactory{}
	mw := AuthUserOrNodeToken(
		fakeIDToken{err: errors.New("must not be called")},
		fakeNodeToken{token: model.NodeToken{NodeID: 7, Permission: model.READ_PERMISSION}},
		f,
	)

	c, w := serve(t, http.MethodGet, "/nodes/7", map[string]string{"Node-Token": id.String()}, mw)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, c)
	_, ok := AuthUserFromContext(c)
	require.False(t, ok, "a node token is not a user")
	access, ok := NodeAccessFromContext(c)
	require.True(t, ok)
	require.Equal(t, 1, f.tokenCalls)
	require.Equal(t, int32(7), f.lastNodeID)
	require.Equal(t, uint8(model.READ_PERMISSION), f.lastPerm)
	bits, err := access.Permission(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, uint8(model.READ_PERMISSION), bits)
}

func TestAuthUserOrNodeTokenRejectsMissingCredentials(t *testing.T) {
	mw := AuthUserOrNodeToken(fakeIDToken{}, fakeNodeToken{}, &fakeFactory{})

	_, w := serve(t, http.MethodGet, "/nodes/1", nil, mw)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthUserOrNodeTokenRejectsInvalidTokenFormat(t *testing.T) {
	mw := AuthUserOrNodeToken(fakeIDToken{}, fakeNodeToken{}, &fakeFactory{})

	_, w := serve(t, http.MethodGet, "/nodes/1", map[string]string{"Node-Token": "not-a-uuid"}, mw)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthUserOrNodeTokenRejectsUnknownToken(t *testing.T) {
	mw := AuthUserOrNodeToken(
		fakeIDToken{},
		fakeNodeToken{err: errors.New("no rows")},
		&fakeFactory{},
	)

	_, w := serve(t, http.MethodGet, "/nodes/1", map[string]string{"Node-Token": uuid.New().String()}, mw)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNodePermissionCheckUsesContextAccess(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		perm       uint8
		stored     uint8
		wantStatus int
	}{
		{name: "read allowed with read bit", method: http.MethodGet, perm: model.READ_PERMISSION + model.WRITE_PERMISSION, stored: model.READ_PERMISSION, wantStatus: http.StatusOK},
		{name: "read denied without read bit", method: http.MethodGet, perm: model.READ_PERMISSION + model.WRITE_PERMISSION, stored: model.WRITE_PERMISSION, wantStatus: http.StatusForbidden},
		{name: "write allowed with full", method: http.MethodPatch, perm: model.READ_PERMISSION + model.WRITE_PERMISSION, stored: model.FULL_PERMISSION, wantStatus: http.StatusOK},
		{name: "grant denied without grant bit", method: http.MethodPost, perm: model.FULL_PERMISSION, stored: model.READ_PERMISSION + model.WRITE_PERMISSION, wantStatus: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setContext := func(c *gin.Context) {
				c.Set(CtxNodeAccess, accessStub{perm: tc.stored})
				c.Set(CtxNodeID, int32(5))
				c.Next()
			}
			_, w := serve(t, tc.method, "/nodes/5", nil, setContext, NodePermissionCheck(tc.perm))
			require.Equal(t, tc.wantStatus, w.Code)
		})
	}
}

func TestNodePermissionCheckUnauthenticated(t *testing.T) {
	setContext := func(c *gin.Context) {
		c.Set(CtxNodeID, int32(5))
		c.Next()
	}
	_, w := serve(t, http.MethodGet, "/nodes/5", nil, setContext, NodePermissionCheck(model.READ_PERMISSION))

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

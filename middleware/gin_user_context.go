package middleware

import (
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/gin-gonic/gin"
)

// Context keys used to pass authenticated and target entities between
// middleware and handlers.
const (
	// CtxAuthUser is the authenticated model.User set by AuthUser.
	CtxAuthUser = "AuthUser"
	// CtxUserID is the :id path target of /api/users/:id routes. It is the
	// user being operated on, which is not necessarily the authenticated user.
	CtxUserID = "UserID"
	// CtxNodeID is the node id extracted from path/body/query or node auth.
	CtxNodeID = "NodeID"
	// CtxNode is reserved for a node entity in the context.
	CtxNode = "Node"
)

// UserFromContext returns the authenticated user stored by AuthUser. The bool
// reports whether a well-typed user is present.
func AuthUserFromContext(c *gin.Context) (model.User, bool) {
	v, ok := c.Get(CtxAuthUser)
	if !ok {
		return model.User{}, false
	}
	u, ok := v.(model.User)
	return u, ok
}

// AuthUserIDFromContext returns the id of the authenticated user.
func AuthUserIDFromContext(c *gin.Context) (int32, bool) {
	u, ok := AuthUserFromContext(c)
	if !ok {
		return 0, false
	}
	return u.ID, true
}

// UserIDFromContext returns the userID
func UserIDFromContext(c *gin.Context) (int32, bool) {
	v, ok := c.Get(CtxUserID)
	if !ok {
		return 0, false
	}
	id, ok := v.(int32)
	return id, ok
}

// NodeIDFromContext returns the node id set by the IdFrom* middleware or node
// auth. The bool reports whether a well-typed id is present.
func NodeIDFromContext(c *gin.Context) (int32, bool) {
	v, ok := c.Get(CtxNodeID)
	if !ok {
		return 0, false
	}
	id, ok := v.(int32)
	return id, ok
}

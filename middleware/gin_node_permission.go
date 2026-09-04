package middleware

import (
	"context"
	"net/http"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/gin-gonic/gin"
)

type NodeAuthorizer interface {
	HasPermission(ctx context.Context, nodeID int32, permission uint8, userID int32) (bool, error)
}

func NodePermissionCheck(a NodeAuthorizer, perm uint8) gin.HandlerFunc {
	return func(c *gin.Context) {

		var p uint8
		if model.Readable(perm) || model.Writeable(perm) {
			p = PermissionFromMethod(c.Request.Method)
		}

		if model.Grantable(perm) {
			p = p + model.GRANT_PERMISSION
		}

		userID, exists := AuthUserIDFromContext(c)
		if !exists {
			AbortWithUnauthorizedError(c, "Unauthenticated")
			return
		}

		nodeID, exists := NodeIDFromContext(c)
		if !exists {
			AbortWithInternalError(c, "NodeID not found")
			return
		}

		b, err := a.HasPermission(c.Request.Context(), nodeID, p, userID)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		if !b {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()

	}
}

func PermissionFromMethod(method string) uint8 {
	switch method {
	case "GET":
		return model.READ_PERMISSION
	case "PUT":
		return model.WRITE_PERMISSION
	case "POST":
		return model.WRITE_PERMISSION
	case "PATCH":
		return model.WRITE_PERMISSION
	case "DELETE":
		return model.WRITE_PERMISSION
	default:
		return 0
	}
}

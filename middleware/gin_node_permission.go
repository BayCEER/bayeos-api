package middleware

import (
	"net/http"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/gin-gonic/gin"
)

// AccessFactory turns an authenticated principal into a model.NodeAccess. It is
// declared here, where it is consumed, and implemented by the service layer.
type AccessFactory interface {
	ForUser(userID int32) model.NodeAccess
	ForToken(nodeID int32, permission uint8) model.NodeAccess
}

func NodePermissionCheck(perm uint8) gin.HandlerFunc {
	return func(c *gin.Context) {

		var p uint8
		if model.Readable(perm) || model.Writeable(perm) {
			p = PermissionFromMethod(c.Request.Method)
		}

		if model.Grantable(perm) {
			p = p + model.GRANT_PERMISSION
		}

		access, exists := NodeAccessFromContext(c)
		if !exists {
			AbortWithUnauthorizedError(c, "Unauthenticated")
			return
		}

		nodeID, exists := NodeIDFromContext(c)
		if !exists {
			AbortWithInternalError(c, "NodeID not found")
			return
		}

		bits, err := access.Permission(c.Request.Context(), nodeID)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		if !model.HasPermission(bits, p) {
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

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

		nodeID, exists := c.Get("NodeID")
		if !exists {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		user, exists := c.Get("User")
		if !exists {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		var p uint8
		if model.Readable(perm) || model.Writeable(perm) {
			p = PermissionFromMethod(c.Request.Method)
		}

		if model.Grantable(perm) {
			p = p + model.GRANT_PERMISSION
		}

		b, err := a.HasPermission(c, nodeID.(int32), p, user.(model.User).ID)
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

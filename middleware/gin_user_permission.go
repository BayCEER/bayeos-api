package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func UserPermissionCheck() gin.HandlerFunc {
	return func(c *gin.Context) {

		user, exists := AuthUserFromContext(c)
		if !exists {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		// Target from path set by middleware
		uID, exists := UserIDFromContext(c)
		if !exists {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		// Permission check
		if !(user.SuperUser || user.ID == uID) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()

	}
}

package middleware

import (
	"net/http"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/gin-gonic/gin"
)

func UserPermissionCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		uID, exists := c.Get("UserID")
		if !exists {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		user, exists := c.Get("User")
		if !exists {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		u := user.(model.User)

		if !(u.SuperUser || u.ID == uID) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()

	}
}

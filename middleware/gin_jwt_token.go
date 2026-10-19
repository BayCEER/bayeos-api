package middleware

import (
	"strings"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-gonic/gin"
)

type authHeader struct {
	Token string `header:"Authorization"`
}

// AuthUser validates the Bearer ID token, stores the user and the matching
// model.NodeAccess in the context. It is the user-only counterpart of
// AuthUserOrNodeToken.
func AuthUser(s service.IDTokenService, f AccessFactory) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := authHeader{}
		if err := c.ShouldBindHeader(&h); err != nil {
			AbortWithBadRequestError(c, "Invalid header information")
			return
		}
		idTokenHeader := strings.Split(h.Token, "Bearer ")
		if len(idTokenHeader) < 2 {
			AbortWithUnauthorizedError(c, "Must provide Authorization header with format `Bearer {token}`")
			return
		}
		user, err := s.ValidateToken(c.Request.Context(), idTokenHeader[1])
		if err != nil {
			AbortWithUnauthorizedError(c, "Provided token is invalid or expired")
			return
		}
		c.Set(CtxAuthUser, user)
		c.Set(CtxNodeAccess, f.ForUser(user.ID))
		c.Next()
	}

}

func AuthSuper() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get(CtxAuthUser)
		if !exists {
			AbortWithInternalError(c, "User not found in context")
			return
		}
		if !user.(model.User).SuperUser {
			AbortWithForbiddenError(c, "Missing privileges")
			return
		}
		c.Next()
	}
}

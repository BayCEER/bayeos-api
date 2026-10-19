package middleware

import (
	"strings"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func NodeBasicAuth(u service.UserService, n service.NodeService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(CtxNode); !exists {
			user, password, hasAuth := c.Request.BasicAuth()
			if hasAuth {
				usr, err := u.SignIn(c.Request.Context(), model.SignInCredentials{Name: user, Password: password})
				if err != nil {
					AbortWithUnauthorizedError(c, "signin failed")
					return
				}
				c.Set(CtxNodeID, usr.HomeNodeID)
			}
		}
		c.Next()
	}
}

type header struct {
	NodeToken string `header:"Node-Token"`
}

// AuthUserOrNodeToken authenticates a request with either a Bearer ID token (a
// user) or a Node-Token header (a node token). It stores the resulting
// model.NodeAccess in the context; it deliberately does not set CtxNodeID, so
// the route's IdFromPath/ResolvePathOrIdFromBody middleware still owns the
// target node.
func AuthUserOrNodeToken(t service.IDTokenService, nt service.NodeTokenService, f AccessFactory) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := authHeader{}
		if err := c.ShouldBindHeader(&h); err == nil {
			if parts := strings.Split(h.Token, "Bearer "); len(parts) == 2 && parts[1] != "" {
				user, err := t.ValidateToken(c.Request.Context(), parts[1])
				if err != nil {
					AbortWithUnauthorizedError(c, "Provided token is invalid or expired")
					return
				}
				c.Set(CtxAuthUser, user)
				c.Set(CtxNodeAccess, f.ForUser(user.ID))
				c.Next()
				return
			}
		}

		nh := header{}
		if err := c.ShouldBindHeader(&nh); err == nil && nh.NodeToken != "" {
			tok, err := uuid.Parse(nh.NodeToken)
			if err != nil {
				AbortWithBadRequestError(c, "invalid token format")
				return
			}
			e, err := nt.FindNodeTokenByValue(c.Request.Context(), tok)
			if err != nil {
				AbortWithUnauthorizedError(c, "Provided node token is invalid or inactive")
				return
			}
			c.Set(CtxNodeAccess, f.ForToken(e.NodeID, e.Permission))
			c.Next()
			return
		}

		AbortWithUnauthorizedError(c, "Must provide an 'Authorization: Bearer {token}' or 'Node-Token' header")
	}
}

package middleware

import (
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func NodeBasicAuth(u service.UserService, n service.NodeService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("Node"); !exists {
			user, password, hasAuth := c.Request.BasicAuth()
			if hasAuth {
				usr, err := u.SignIn(c, model.SignInCredentials{Name: user, Password: password})
				if err != nil {
					AbortWithUnauthorizedError(c, "signin failed")
					return
				}
				c.Set("NodeID", usr.HomeNodeID)
			}
		}
		c.Next()
	}
}

type header struct {
	NodeToken string `header:"Node-Token"`
}

func NodeToken(s service.NodeService) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if _, exists := ctx.Get("NodeID"); !exists {
			h := header{}
			if err := ctx.ShouldBindHeader(&h); err == nil {
				if t, err := uuid.Parse(h.NodeToken); err == nil {
					if n, err := s.FindNodeByToken(ctx, t); err == nil {
						ctx.Set("NodeID", n.ID)
					} else {
						AbortWithNotFoundError(ctx, "node", "token", h.NodeToken)
						return
					}
				} else {
					AbortWithBadRequestError(ctx, "invalid token format")
					return
				}
			}
		}
		ctx.Next()
	}
}

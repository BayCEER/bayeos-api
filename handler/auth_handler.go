package handler

import (
	"fmt"
	"net/http"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/gin-gonic/gin"
)

func (h *Handler) SignIn(c *gin.Context) {
	var cre model.SignInCredentials

	if err := c.BindJSON(&cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid sign in values.")
		return
	}

	u, err := h.UserService.SignIn(c, cre)
	if err != nil {
		mw.AbortWithBadRequestError(c, "Signin failed")
		return
	}

	// Token
	token, err := h.IDTokenService.CreateToken(u)
	if err != nil {
		mw.AbortWithInternalError(c, "Token creation failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": token,
	})

}

func (h *Handler) SignUp(c *gin.Context) {
	var cre model.SignUpCredentials
	if err := c.ShouldBind(&cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid sign up values.")
		return
	}
	_, err := h.UserService.CreateUser(c, cre)
	if err != nil {
		mw.AbortWithInternalError(c, "Sign up failed.")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "User created"})
}

func (h *Handler) SignOut(c *gin.Context) {
	var cre model.SignInCredentials

	if err := c.BindJSON(&cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid sign out values.")
		return
	}
	err := h.UserService.SignOut(c, cre)
	if err != nil {
		mw.AbortWithInternalError(c, "Signout failed.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User signed out"})
}

func (h *Handler) SetPassword(c *gin.Context) {
	user, exists := c.Get("User")
	if !exists {
		mw.AbortWithUnauthorizedError(c, "Missing header information")
		return
	}

	var sc model.SignInCredentials
	if err := c.ShouldBind(&sc); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid body.")
		return
	}

	if !user.(model.User).SuperUser && sc.Name != user.(model.User).SignInName {
		mw.AbortWithForbiddenError(c, "Missing privileges")
		return
	}

	if err := h.UserService.SetPassword(c, sc); err != nil {
		mw.AbortWithInternalError(c, "Failed to set password")
		return

	}
	c.JSON(http.StatusOK, gin.H{"message": "Password changed"})
}

func (h *Handler) RefreshIDToken(c *gin.Context) {
	user, exists := c.Get("User")
	if !exists {
		mw.AbortWithBadRequestError(c, "Missing header information")
		return
	}
	token, err := h.IDTokenService.CreateToken(user.(model.User))
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to create token for user: %v", user.(model.User).SignInName))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": token,
	})
}

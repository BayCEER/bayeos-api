package handler

import (
	"fmt"
	"net/http"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/strings"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetNodePrivs(c *gin.Context) {

	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	privs, err := h.NodeService.GetNodePrivs(c.Request.Context(), id)
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get node privileges: %v", err))
		return
	}
	c.JSON(http.StatusOK, privs)

}

func (h *Handler) GetNodePriv(c *gin.Context) {

	pid, err := strings.ParseStringToInt32(c.Param("pid"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if np, err := h.NodePrivService.GetNodePriv(c.Request.Context(), pid); err != nil {
			mw.AbortWithNotFoundError(c, "NodePrivilege", "pid", c.Param("pid"))
			return
		} else {
			c.JSON(http.StatusOK, np)
		}
	}

}

func (h *Handler) CreateNodePriv(c *gin.Context) {
	p := model.NodePrivilege{}

	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	user, exists := c.Get("User")
	if !exists {
		mw.AbortWithUnauthorizedError(c, "Missing header information")
		return
	}
	if err := c.BindJSON(&p); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}

	// Overwrite body id, grantor is the authenticated user
	p.NodeID = id
	p.GrantorID = user.(model.User).ID

	if err := h.NodePrivService.CreateNodePriv(c.Request.Context(), &p); err != nil {
		mw.AbortWithInternalError(c, "Failed to create node privilege")
		return
	}

	c.JSON(http.StatusCreated, p)

}

func (h *Handler) DeleteNodePrivilege(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("pid")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if err := h.NodePrivService.DeleteNodePriv(c.Request.Context(), id); err != nil {
			if mw.AbortWithNotFoundIfNoRows(c, err, "NodePrivilege", "pid", c.Param("pid")) {
				return
			}
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to delete node privilege: %v", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Node privilege deleted"})
	}
}

func (h *Handler) UpdateNodePriv(c *gin.Context) {
	pid, err := strings.ParseStringToInt32(c.Param("pid"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "invalid query parameter")
		return
	}

	p := model.NodePrivilege{}
	if err := c.BindJSON(&p); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}

	if err := h.NodePrivService.UpdateNodePriv(c.Request.Context(), pid, p.Permission); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to update node privilege: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Node privilege updated"})

}

func (h *Handler) GetNodePrivsByGrantorID(c *gin.Context) {

	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	privs, err := h.NodePrivService.GetNodePrivsByGrantorID(c.Request.Context(), id)
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get node privileges: %v", err))
		return
	}
	c.JSON(http.StatusOK, privs)

}

func (h *Handler) GetNodePrivsByGranteeID(c *gin.Context) {

	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	privs, err := h.NodePrivService.GetNodePrivsByGranteeID(c.Request.Context(), id)
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get node privileges: %v", err))
		return
	}
	c.JSON(http.StatusOK, privs)

}

package handler

import (
	"encoding/json"
	"fmt"
	"io"

	"net/http"
	"strconv"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/strings"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetNode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if err, userID := h.getUserID(c); err != nil {
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get user id: %v", err))
			return
		} else {
			if n, err := h.NodeService.GetNode(c.Request.Context(), int32(id), userID); err != nil {
				mw.AbortWithNotFoundError(c, "node", "id", c.Param("id"))
				return
			} else {
				c.JSON(http.StatusOK, n)
			}
		}
	}
}

func (h *Handler) GetNodes(c *gin.Context) {
	var query model.IdFormQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid query parameters")
		return
	}

	if err, userID := h.getUserID(c); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get user id: %v", err))
		return
	} else {
		if nodes, err := h.NodeService.GetNodes(c.Request.Context(), query.IDs, userID); err != nil {
			mw.AbortWithInternalError(c, "Nodes not found")
			return
		} else {
			c.JSON(http.StatusOK, nodes)
		}

	}

}

func (h *Handler) GetNodeChildren(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}

	if err, userID := h.getUserID(c); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get user id: %v", err))
		return
	} else {
		if n, err := h.NodeService.GetNodeChildren(c.Request.Context(), int32(id), userID); err != nil {
			mw.AbortWithNotFoundError(c, "node", "id", c.Param("id"))
			return
		} else {
			c.JSON(http.StatusOK, n)
		}
	}
}

func (h *Handler) GetNodeParents(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}

	if err, userID := h.getUserID(c); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get user id: %v", err))
		return
	} else {
		if n, err := h.NodeService.GetNodeParents(c.Request.Context(), int32(id), userID); err != nil {
			mw.AbortWithNotFoundError(c, "node", "id", c.Param("id"))
		} else {
			c.JSON(http.StatusOK, n)
		}
	}
}

func (h *Handler) SearchNodes(c *gin.Context) {

	query := c.DefaultQuery("query", "")
	skip, err := strconv.Atoi(c.DefaultQuery("skip", "0"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid skip parameter")
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid limit parameter")
		return
	}
	err, userID := h.getUserID(c)
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get user id: %v", err))
		return
	}
	if total, nodes, err := h.NodeService.FindNodesByPath(c.Request.Context(), query, skip, limit, userID); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Search failed: %v", err))
	} else {
		c.JSON(http.StatusOK, gin.H{"total": total, "hits": nodes})
	}

}

func (h *Handler) CreateNode(c *gin.Context) {
	d := model.Node{}
	if err := c.BindJSON(&d); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}

	err, userID := h.getUserID(c)
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get user id: %v", err))
		return
	}

	if err := h.NodeService.CreateNode(c.Request.Context(), &d, userID); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to create node: %v", err))
		return
	}
	c.JSON(http.StatusCreated, d)

}

func (h *Handler) DeleteNode(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
	} else {
		if err := h.NodeService.DeleteNode(c.Request.Context(), id); err != nil {
			if mw.AbortWithNotFoundIfNoRows(c, err, "node", "id", c.Param("id")) {
				return
			}
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to delete node: %v", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Node deleted"})
	}
}

func (h *Handler) UpdateNode(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
	} else {

		jsonData, err := io.ReadAll(c.Request.Body)
		if err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")

			return
		}

		// Validate body
		if err := ValidateBody(jsonData, &model.Node{}); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")

			return
		}

		// Set field values
		var fields map[string]interface{}
		if err := json.Unmarshal(jsonData, &fields); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}
		if err := h.NodeService.UpdateNode(c.Request.Context(), id, &fields); err != nil {
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to update node: %v", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Node updated"})
	}
}

func (h *Handler) getUserID(c *gin.Context) (error, int32) {
	user, exists := c.Get("User")
	if !exists {
		return fmt.Errorf("user not found in context"), 0
	}
	u := user.(model.User)
	return nil, u.ID
}

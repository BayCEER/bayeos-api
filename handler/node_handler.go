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
		if n, err := h.NodeService.GetNode(c, int32(id)); err != nil {
			mw.AbortWithNotFoundError(c, "node", "id", c.Param("id"))
			return
		} else {
			c.JSON(http.StatusOK, n)
		}
	}
}

func (h *Handler) GetNodes(c *gin.Context) {
	var query model.IdFormQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid query parameters")
		return
	}
	if nodes, err := h.NodeService.GetNodes(c, query.IDs); err != nil {
		mw.AbortWithInternalError(c, "Nodes not found")
		return
	} else {
		c.JSON(http.StatusOK, nodes)
	}

}

func (h *Handler) GetNodeChildren(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if n, err := h.NodeService.GetNodeChildren(c, int32(id)); err != nil {
			mw.AbortWithNotFoundError(c, "node", "id", c.Param("id"))
			return
		} else {
			c.JSON(http.StatusOK, n)
		}
	}
}

func (h *Handler) GetRootNode(c *gin.Context) {
	if n, err := h.NodeService.GetNode(c, model.ROOT_ID); err != nil {
		mw.AbortWithNotFoundError(c, "node", "id", fmt.Sprintf("%d", model.ROOT_ID))
		return
	} else {
		c.JSON(http.StatusOK, n)
	}

}

func (h *Handler) GetNodeParents(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if n, err := h.NodeService.GetNodeParents(c, int32(id)); err != nil {
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
	if total, nodes, err := h.NodeService.FindNodesByPath(c, query, skip, limit); err != nil {
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

	if err := h.NodeService.CreateNode(c, &d); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to create node: %v", err))
		return
	}
	c.JSON(http.StatusCreated, d)

}

func (h *Handler) DeleteNode(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
	} else {
		if err := h.NodeService.DeleteNode(c, id); err != nil {
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
		if err := h.NodeService.UpdateNode(c, id, &fields); err != nil {
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to update node: %v", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Node updated"})
	}
}

package handler

import (
	"database/sql"
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
		access, ok := mw.NodeAccessFromContext(c)
		if !ok {
			mw.AbortWithUnauthorizedError(c, "unauthenticated")
			return
		}
		if n, err := h.NodeService.GetNode(c.Request.Context(), int32(id), access); err != nil {
			mw.AbortWithNotFoundError(c, "node", "id", c.Param("id"))
			return
		} else {
			c.JSON(http.StatusOK, n)
		}
	}
}

func (h *Handler) GetNodes(c *gin.Context) {
	ids, err := strings.ParseCSVToInt32(c.Query("ids"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid ids parameter")
		return
	}

	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	if nodes, err := h.NodeService.GetNodes(c.Request.Context(), ids, access); err != nil {
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
	}

	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	if n, err := h.NodeService.GetNodeChildren(c.Request.Context(), int32(id), access); err != nil {
		mw.AbortWithNotFoundError(c, "node", "id", c.Param("id"))
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
	}

	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	if n, err := h.NodeService.GetNodeParents(c.Request.Context(), int32(id), access); err != nil {
		mw.AbortWithNotFoundError(c, "node", "id", c.Param("id"))
	} else {
		c.JSON(http.StatusOK, n)
	}
}

func (h *Handler) SearchNodesByAntPath(c *gin.Context) {
	pattern := c.DefaultQuery("pattern", "")
	if pattern == "" {
		mw.AbortWithBadRequestError(c, "Invalid pattern parameter")
		return
	}
	kind := c.DefaultQuery("kind", "")
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
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	if total, nodes, err := h.NodeService.FindNodesByAntPath(c.Request.Context(), pattern, kind, skip, limit, access); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Search failed: %v", err))
	} else {
		c.JSON(http.StatusOK, gin.H{"total": total, "hits": nodes})
	}
}

func (h *Handler) CreateNode(c *gin.Context) {
	var body struct {
		DisplayName string         `json:"display_name" binding:"required"`
		Kind        string         `json:"kind"`
		ParentID    *int32         `json:"parent_id"`
		Path        *string        `json:"node_path"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := c.BindJSON(&body); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}

	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}

	d := model.Node{
		DisplayName: body.DisplayName,
		Kind:        body.Kind,
	}
	if body.Metadata != nil {
		md, err := model.MetadataFromMap(body.Metadata)
		if err != nil {
			mw.AbortWithBadRequestError(c, "Invalid metadata")
			return
		}
		d.Metadata = md
	}

	if body.Path != nil && *body.Path != "" {
		// Path-based creation: middleware already resolved the base node
		// and set NodeID. Strip the prefix to get the relative path.
		baseID, _ := mw.NodeIDFromContext(c)
		rel := *body.Path
		if len(rel) > 0 && (rel[0] == '/' || rel[0] == '~') {
			rel = rel[1:]
		}
		if err := h.NodeService.CreateNodeByPath(c.Request.Context(), &d, baseID, rel, access); err != nil {
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to create node: %v", err))
			return
		}
	} else {
		if body.ParentID == nil {
			mw.AbortWithBadRequestError(c, "either path or parent_id is required")
			return
		}
		d.ParentID = *body.ParentID
		if err := h.NodeService.CreateNode(c.Request.Context(), &d, access); err != nil {
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to create node: %v", err))
			return
		}
	}
	c.JSON(http.StatusCreated, d)
}

func (h *Handler) DeleteNode(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
	} else {
		// Collect all node IDs that will be removed (the node itself plus
		// every descendant) before any rows are deleted.
		nodeIDs, err := h.NodeService.FindNodeDescendantsByID(c.Request.Context(), id)
		if err != nil {
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to find descendants: %v", err))
			return
		}
		if len(nodeIDs) == 0 {
			mw.AbortWithNotFoundIfNoRows(c, sql.ErrNoRows, "node", "id", c.Param("id"))
			return
		}

		// Explicitly delete all collected nodes (does not rely on ON DELETE CASCADE).
		if err := h.NodeService.DeleteNodesByIDs(c.Request.Context(), nodeIDs); err != nil {
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to delete nodes: %v", err))
			return
		}

		// Notify plugins (e.g. QuestDB cleanup) with the full set of deleted IDs.
		// h.runNodeDeleteHooks(c.Request.Context(), nodeIDs)

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
			if mw.AbortWithBadRequestIfReadOnlyField(c, err) {
				return
			}
			if mw.AbortWithBadRequestIfComputedNotWritable(c, err) {
				return
			}
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to update node: %v", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Node updated"})
	}
}

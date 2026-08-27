package handler

import (
	"encoding/json"
	"io"
	"net/http"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/strings"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) GetNodeTokens(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	tokens, err := h.NodeService.GetNodeTokens(c, id)
	if err != nil {
		mw.AbortWithInternalError(c, "Failed to get node tokens")
		return
	}
	c.JSON(http.StatusOK, tokens)
}

func (h *Handler) GetNodeToken(c *gin.Context) {
	nid, err := strings.ParseStringToInt32(c.Param("nid"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "invalid query parameter")
		return
	} else {
		if nt, err := h.NodeTokenService.GetNodeToken(c, nid); err != nil {
			mw.AbortWithNotFoundError(c, "NodeToken", "nid", c.Param("nid"))
			return
		} else {
			c.JSON(http.StatusOK, nt)
		}
	}
}

func (h *Handler) CreateNodeToken(c *gin.Context) {

	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "invalid query parameter")
		return
	} else {
		// Empty body !
		nt := model.NodeToken{}
		nt.NodeID = id
		nt.Token = uuid.New()
		if err := h.NodeTokenService.CreateNodeToken(c, &nt); err != nil {
			mw.AbortWithInternalError(c, "Failed to create node token")
			return
		} else {
			c.JSON(http.StatusCreated, nt)
		}

	}
}

func (h *Handler) DeleteNodeToken(c *gin.Context) {
	if nid, err := strings.ParseStringToInt32(c.Param("nid")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
	} else {
		if err := h.NodeTokenService.DeleteNodeToken(c, nid); err != nil {
			if mw.AbortWithNotFoundIfNoRows(c, err, "NodeToken", "nid", c.Param("nid")) {
				return
			}
			mw.AbortWithInternalError(c, "Failed to delete node token")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "NodeToken deleted"})
	}
}

func (h *Handler) UpdateNodeToken(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {

		jsonData, err := io.ReadAll(c.Request.Body)
		if err != nil {
			mw.AbortWithBadRequestError(c, "Could not read request body")
			return
		}

		// Validate body
		if err := ValidateBody(jsonData, &model.NodeToken{}); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}

		// Set field values
		var fields map[string]interface{}
		if err := json.Unmarshal(jsonData, &fields); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid field data in request body")
			return
		}
		if err := h.NodeTokenService.UpdateNodeToken(c, id, &fields); err != nil {
			mw.AbortWithInternalError(c, "Failed to update node token")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "NodeToken updated"})
	}
}

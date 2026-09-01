package timeseries

import (
	"errors"
	"fmt"
	"net/http"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/strings"
	"github.com/gin-gonic/gin"
)

// Handlers bundles the timeseries HTTP handlers.
type Handlers struct {
	service TimeseriesService
}

// NewHandlers returns route handlers for the timeseries plugin.
func NewHandlers(s TimeseriesService) *Handlers {
	return &Handlers{service: s}
}

func (h *Handlers) userId(c *gin.Context) (int32, bool) {
	u, exists := c.Get("User")
	if !exists {
		return 0, false
	}
	return u.(model.User).ID, true
}

func (h *Handlers) Create(c *gin.Context) {
	userID, ok := h.userId(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "User not found in context")
		return
	}
	var cre model.Node
	if err := c.ShouldBindJSON(&cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	n, err := h.service.Create(c.Request.Context(), TimeseriesCreate{DisplayName: cre.DisplayName, ParentID: cre.ParentID}, userID)
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to create timeseries: %v", err))
		return
	}
	c.JSON(http.StatusCreated, n)
}

func (h *Handlers) Get(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	userID, ok := h.userId(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "User not found in context")
		return
	}
	n, err := h.service.Get(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
		return
	}
	c.JSON(http.StatusOK, n)
}

func (h *Handlers) Update(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	userID, ok := h.userId(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "User not found in context")
		return
	}
	var fields map[string]interface{}
	if err := c.ShouldBindJSON(&fields); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	if err := h.service.Update(c.Request.Context(), id, fields, userID); err != nil {
		if errors.Is(err, ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to update timeseries: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Timeseries updated"})
}

func (h *Handlers) Delete(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	userID, ok := h.userId(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "User not found in context")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to delete timeseries: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Timeseries deleted"})
}

func (h *Handlers) AddData(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	userID, ok := h.userId(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "User not found in context")
		return
	}
	var pts []DataPoint
	if err := c.ShouldBindJSON(&pts); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	if err := h.service.AddData(c.Request.Context(), id, pts, userID); err != nil {
		if errors.Is(err, ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to insert data: %v", err))
		return
	}
	c.Status(http.StatusCreated)
}

func (h *Handlers) QueryData(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	userID, ok := h.userId(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "User not found in context")
		return
	}
	var q TimeseriesQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid query parameters")
		return
	}
	pts, err := h.service.QueryData(c.Request.Context(), id, q, userID)
	if err != nil {
		if errors.Is(err, ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to query data: %v", err))
		return
	}
	c.JSON(http.StatusOK, pts)
}
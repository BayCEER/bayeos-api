package timeseries

import (
	"errors"
	"fmt"
	"net/http"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
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

func (h *Handlers) Create(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}
	baseID, _ := mw.NodeIDFromContext(c)
	var cre TimeseriesCreate
	if err := c.ShouldBindJSON(&cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	n, err := h.service.Create(c.Request.Context(), cre, baseID, userID)
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
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
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
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
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
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
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
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
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

func (h *Handlers) AddDataBatch(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}
	var req SeriesDataBatch
	if err := c.ShouldBindJSON(&req); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	if len(req.Series) == 0 {
		mw.AbortWithBadRequestError(c, "at least one series is required")
		return
	}
	if err := h.service.AddDataBatch(c.Request.Context(), req.Series, userID); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to insert data: %v", err))
		return
	}
	c.Status(http.StatusCreated)
}

func (h *Handlers) UpdateStatus(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}
	var pts []StatusPoint
	if err := c.ShouldBindJSON(&pts); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	if len(pts) == 0 {
		mw.AbortWithBadRequestError(c, "at least one status record is required")
		return
	}
	if err := h.service.UpdateStatus(c.Request.Context(), id, pts, userID); err != nil {
		if errors.Is(err, ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to update status: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}

func (h *Handlers) QueryData(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}
	var q TimeseriesQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid query parameters")
		return
	}
	data, err := h.service.QueryData(c.Request.Context(), id, q, userID)
	if err != nil {
		if errors.Is(err, ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to query data: %v", err))
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *Handlers) QueryDataBatch(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}
	var q TimeseriesDataQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid query parameters")
		return
	}
	ids, err := strings.ParseCSVToInt32(c.Query("ids"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid ids parameter")
		return
	}
	q.IDs = ids
	if len(q.IDs) == 0 {
		mw.AbortWithBadRequestError(c, "at least one ids value is required")
		return
	}
	data, err := h.service.QueryDataBatch(c.Request.Context(), q, userID)
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to query data: %v", err))
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *Handlers) GetStats(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}
	var q TimeseriesStatsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid query parameters")
		return
	}
	stats, err := h.service.GetStats(c.Request.Context(), id, q, userID)
	if err != nil {
		if errors.Is(err, ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to compute stats: %v", err))
		return
	}
	c.JSON(http.StatusOK, stats)
}

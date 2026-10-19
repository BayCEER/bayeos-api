package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/BayCEER/bayeos/bayeos-api/strings"
	"github.com/gin-gonic/gin"
)

func (h *Handler) CreateTimeseries(c *gin.Context) {
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	baseID, _ := mw.NodeIDFromContext(c)
	var cre model.TimeseriesCreate
	if err := c.ShouldBindJSON(&cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	n, err := h.TimeseriesService.Create(c.Request.Context(), cre, baseID, access)
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to create timeseries: %v", err))
		return
	}
	c.JSON(http.StatusCreated, n)
}

func (h *Handler) GetTimeseries(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	n, err := h.TimeseriesService.Get(c.Request.Context(), id, access)
	if err != nil {
		if errors.Is(err, service.ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
		return
	}
	c.JSON(http.StatusOK, n)
}

func (h *Handler) UpdateTimeseries(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	var fields map[string]interface{}
	if err := c.ShouldBindJSON(&fields); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	if err := h.TimeseriesService.Update(c.Request.Context(), id, fields, access); err != nil {
		if errors.Is(err, service.ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		if mw.AbortWithBadRequestIfComputedNotWritable(c, err) {
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to update timeseries: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Timeseries updated"})
}

func (h *Handler) DeleteTimeseries(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	if err := h.TimeseriesService.Delete(c.Request.Context(), id, access); err != nil {
		if errors.Is(err, service.ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to delete timeseries: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Timeseries deleted"})
}

func (h *Handler) AddTimeseriesData(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	var pts []model.DataPoint
	if err := c.ShouldBindJSON(&pts); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	ignoreInvalid := queryBoolDefault(c, "ignore_invalid_timestamps", false)
	if err := h.TimeseriesService.AddData(c.Request.Context(), id, pts, access, ignoreInvalid); err != nil {
		if errors.Is(err, service.ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to insert data: %v", err))
		return
	}
	c.Status(http.StatusCreated)
}

func (h *Handler) AddTimeseriesDataBatch(c *gin.Context) {
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	var req model.SeriesDataBatch
	if err := c.ShouldBindJSON(&req); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	if len(req.Series) == 0 {
		mw.AbortWithBadRequestError(c, "at least one series is required")
		return
	}
	ignoreInvalid := queryBoolDefault(c, "ignore_invalid_timestamps", false)
	if err := h.TimeseriesService.AddDataBatch(c.Request.Context(), req.Series, access, ignoreInvalid); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to insert data: %v", err))
		return
	}
	c.Status(http.StatusCreated)
}

func (h *Handler) QueryTimeseriesData(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	var q model.TimeseriesQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid query parameters")
		return
	}
	statusFilter, err := queryStatusFilter(c)
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid status_filter parameter")
		return
	}
	q.StatusFilter = statusFilter
	data, err := h.TimeseriesService.QueryData(c.Request.Context(), id, q, access)
	if err != nil {
		if errors.Is(err, service.ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to query data: %v", err))
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *Handler) QueryTimeseriesDataBatch(c *gin.Context) {
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	var q model.TimeseriesDataQuery
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
	statusFilter, err := queryStatusFilter(c)
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid status_filter parameter")
		return
	}
	q.StatusFilter = statusFilter
	data, err := h.TimeseriesService.QueryDataBatch(c.Request.Context(), q, access)
	if err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to query data: %v", err))
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *Handler) GetTimeseriesStats(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	var q model.TimeseriesStatsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid query parameters")
		return
	}
	stats, err := h.TimeseriesService.GetStats(c.Request.Context(), id, q, access)
	if err != nil {
		if errors.Is(err, service.ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to compute stats: %v", err))
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) GetTimeseriesGaps(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}
	access, ok := mw.NodeAccessFromContext(c)
	if !ok {
		mw.AbortWithUnauthorizedError(c, "unauthenticated")
		return
	}
	var q model.TimeseriesGapsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid query parameters")
		return
	}
	gaps, err := h.TimeseriesService.GetGaps(c.Request.Context(), id, q, access)
	if err != nil {
		if errors.Is(err, service.ErrNotTimeseries) {
			mw.AbortWithNotFoundError(c, "timeseries", "id", c.Param("id"))
			return
		}
		if errors.Is(err, service.ErrNoResolution) {
			mw.AbortWithBadRequestError(c, "node metadata requires a resolution value, or provide threshold_minutes")
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to detect gaps: %v", err))
		return
	}
	c.JSON(http.StatusOK, gaps)
}

// queryBoolDefault returns the boolean query parameter key, or def when the
// parameter is empty or cannot be parsed as a bool.
func queryBoolDefault(c *gin.Context, key string, def bool) bool {
	raw := c.DefaultQuery(key, "")
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return b
}

// queryStatusFilter parses the comma-separated "status_filter" query parameter
// into a status slice. An absent or empty parameter returns an empty slice,
// which the service resolves to the default [0,1,2].
func queryStatusFilter(c *gin.Context) ([]model.Status, error) {
	raw, err := strings.ParseCSVToInt32(c.Query("status_filter"))
	if err != nil {
		return nil, err
	}
	filter := make([]model.Status, len(raw))
	for i, v := range raw {
		filter[i] = model.Status(v)
	}
	return filter, nil
}

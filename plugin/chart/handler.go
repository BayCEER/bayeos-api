package chart

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/strings"
	"github.com/gin-gonic/gin"
)

// ChartHandlers bundles the chart HTTP handlers.
type ChartHandlers struct {
	service ChartService
}

// NewChartHandlers returns route handlers for the chart extension.
func NewChartHandlers(s ChartService) *ChartHandlers {
	return &ChartHandlers{service: s}
}

func (h *ChartHandlers) GetCharts(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}

	pattern := c.DefaultQuery("pattern", "")
	skip, err := strconv.Atoi(c.DefaultQuery("skip", "0"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid skip parameter")
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "0"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid limit parameter")
		return
	}

	if charts, total, err := h.service.GetCharts(c.Request.Context(), userID, pattern, skip, limit); err != nil {
		mw.AbortWithInternalError(c, "Failed to get charts")
	} else {
		c.JSON(http.StatusOK, model.Page[UserChart]{Total: total, Hits: charts})
	}
}

func (h *ChartHandlers) GetChart(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}

	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
	} else {
		if chart, err := h.service.GetChart(c.Request.Context(), id, userID); err != nil {
			mw.AbortWithNotFoundError(c, "Chart", "id", c.Param("id"))
		} else {
			c.JSON(http.StatusOK, chart)
		}
	}
}

func (h *ChartHandlers) CreateChart(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}

	jsonData, err := io.ReadAll(c.Request.Body)
	if err != nil {
		mw.AbortWithBadRequestError(c, "Could not read request body")
		return
	}

	// Validate body
	if err := validateBody(jsonData, &UserChartCreate{}); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}

	var cre UserChartCreate
	if err := json.Unmarshal(jsonData, &cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}

	if chart, err := h.service.CreateChart(c.Request.Context(), userID, cre); err != nil {
		if errors.Is(err, ErrChartNameTaken) {
			mw.Abort(c, mw.New(http.StatusConflict, "Conflict", "A chart with this name already exists"))
			return
		}
		mw.AbortWithInternalError(c, "Failed to create chart")
		return
	} else {
		c.JSON(http.StatusCreated, chart)
	}
}

func (h *ChartHandlers) UpdateChart(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}

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
		if err := validateBody(jsonData, &UserChartUpdate{}); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}

		var upd UserChartUpdate
		if err := json.Unmarshal(jsonData, &upd); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}

		if err := h.service.UpdateChart(c.Request.Context(), id, userID, upd); err != nil {
			if mw.AbortWithBadRequestIfReadOnlyField(c, err) {
				return
			}
			if errors.Is(err, ErrChartNameTaken) {
				mw.Abort(c, mw.New(http.StatusConflict, "Conflict", "A chart with this name already exists"))
				return
			}
			if mw.AbortWithNotFoundIfNoRows(c, err, "Chart", "id", c.Param("id")) {
				return
			}
			mw.AbortWithInternalError(c, "Failed to update chart")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Chart updated"})
	}
}

func (h *ChartHandlers) DeleteChart(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}

	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
	} else {
		if err := h.service.DeleteChart(c.Request.Context(), id, userID); err != nil {
			if mw.AbortWithNotFoundIfNoRows(c, err, "Chart", "id", c.Param("id")) {
				return
			}
			mw.AbortWithInternalError(c, "Failed to delete chart")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Chart deleted"})
	}
}

// validateBody decodes obj from body, rejecting unknown fields.
func validateBody(body []byte, obj any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(obj)
}

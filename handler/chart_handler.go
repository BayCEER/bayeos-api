package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/BayCEER/bayeos/bayeos-api/strings"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetCharts(c *gin.Context) {
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

	if charts, total, err := h.ChartService.GetCharts(c.Request.Context(), userID, pattern, skip, limit); err != nil {
		mw.AbortWithInternalError(c, "Failed to get charts")
	} else {
		c.JSON(http.StatusOK, model.Page[model.UserChart]{Total: total, Hits: charts})
	}
}

func (h *Handler) GetChart(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}

	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
	} else {
		if chart, err := h.ChartService.GetChart(c.Request.Context(), id, userID); err != nil {
			mw.AbortWithNotFoundError(c, "Chart", "id", c.Param("id"))
		} else {
			c.JSON(http.StatusOK, chart)
		}
	}
}

func (h *Handler) CreateChart(c *gin.Context) {
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
	if err := validateBody(jsonData, &model.UserChartCreate{}); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}

	var cre model.UserChartCreate
	if err := json.Unmarshal(jsonData, &cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}

	if chart, err := h.ChartService.CreateChart(c.Request.Context(), userID, cre); err != nil {
		if errors.Is(err, service.ErrChartNameTaken) {
			mw.Abort(c, mw.New(http.StatusConflict, "Conflict", "A chart with this name already exists"))
			return
		}
		mw.AbortWithInternalError(c, "Failed to create chart")
		return
	} else {
		c.JSON(http.StatusCreated, chart)
	}
}

func (h *Handler) UpdateChart(c *gin.Context) {
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
		if err := validateBody(jsonData, &model.UserChartUpdate{}); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}

		var upd model.UserChartUpdate
		if err := json.Unmarshal(jsonData, &upd); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}

		if err := h.ChartService.UpdateChart(c.Request.Context(), id, userID, upd); err != nil {
			if mw.AbortWithBadRequestIfReadOnlyField(c, err) {
				return
			}
			if errors.Is(err, service.ErrChartNameTaken) {
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

func (h *Handler) DeleteChart(c *gin.Context) {
	userID, ok := mw.AuthUserIDFromContext(c)
	if !ok {
		mw.AbortWithInternalError(c, "user id not found")
		return
	}

	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
	} else {
		if err := h.ChartService.DeleteChart(c.Request.Context(), id, userID); err != nil {
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

package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/strings"
	"github.com/gin-gonic/gin"
)

func IdFromBody(bodyKey string, ctxKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(ctxKey); !exists {
			buf, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
				return
			}
			rdr := io.NopCloser(bytes.NewBuffer(buf))
			c.Request.Body = io.NopCloser(bytes.NewBuffer(buf))
			bts, err := io.ReadAll(rdr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
				return
			}
			var body interface{}
			if e := json.Unmarshal(bts, &body); e != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, e.Error())
				return
			}
			m := body.(map[string]interface{})

			val, ok := m[bodyKey]
			if !ok {
				c.AbortWithStatusJSON(http.StatusBadRequest, "invalid key value ")
				return
			}

			c.Set(ctxKey, int32(val.(float64)))
		}
		c.Next()
	}
}

func IdFromQuery(queryKey string, ctxKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(ctxKey); !exists {
			if id, err := strings.ParseStringToInt32(c.Query(queryKey)); err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
				return
			} else {
				c.Set(ctxKey, int32(id))
			}
		}
		c.Next()
	}
}

func IdFromPath(pathKey string, ctxKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(ctxKey); !exists {
			if id, err := strings.ParseStringToInt32(c.Param(pathKey)); err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
				return
			} else {
				c.Set(ctxKey, int32(id))
			}
		}
		c.Next()
	}
}

func IdFromConst(id int32, ctxKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(ctxKey); !exists {
			c.Set(ctxKey, id)
		}
		c.Next()
	}
}

func PermissionFromMethod(method string) uint8 {
	switch method {
	case "GET":
		return model.READ_PERMISSION
	case "PUT":
		return model.WRITE_PERMISSION
	case "POST":
		return model.WRITE_PERMISSION
	case "PATCH":
		return model.WRITE_PERMISSION
	case "DELETE":
		return model.WRITE_PERMISSION
	default:
		return 0
	}
}

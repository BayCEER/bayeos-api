package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

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

// ResolvePathOrIdFromBody reads the JSON body and sets ctxKey to either:
//   - the base node resolved from a "path" field ( "/" → root, "~" → home), or
//   - the integer from bodyKey (e.g. "parent_id").
//
// It aborts 400 if neither key is present. The body is cloned so downstream
// handlers and middleware can re-read it.
func ResolvePathOrIdFromBody(bodyKey, pathKey, ctxKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(ctxKey); exists {
			c.Next()
			return
		}
		buf, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(buf))

		var body map[string]interface{}
		if e := json.Unmarshal(buf, &body); e != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, "invalid JSON body")
			return
		}

		if rawPath, ok := body[pathKey]; ok {
			if path, ok := rawPath.(string); ok && path != "" {
				var baseID int32
				switch path[0] {
				case '/':
					baseID = 1 // model.ROOT_ID
				case '~':
					user, ok := AuthUserFromContext(c)
					if !ok {
						AbortWithUnauthorizedError(c, "unauthenticated")
						return
					}
					baseID = user.HomeNodeID
				default:
					c.AbortWithStatusJSON(http.StatusBadRequest, "path must start with / or ~")
					return
				}
				c.Set(ctxKey, baseID)
				c.Next()
				return
			}
			// pathKey is present but empty -> fall through to bodyKey.
		}

		if val, ok := body[bodyKey]; ok {
			c.Set(ctxKey, int32(val.(float64)))
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusBadRequest, "either path or parent_id is required")
	}
}

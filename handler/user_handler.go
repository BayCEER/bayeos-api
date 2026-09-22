package handler

import (
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

func (h *Handler) CreateUser(c *gin.Context) {
	var cre model.SignUpCredentials
	if err := c.BindJSON(&cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid sign up values.")
		return
	}
	u, err := h.UserService.CreateUser(c.Request.Context(), cre)
	if err != nil {
		if errors.Is(err, service.ErrSignInNameTaken) {
			mw.Abort(c, mw.New(http.StatusConflict, "Conflict", "A user with this sign in name already exists"))
			return
		}
		mw.AbortWithInternalError(c, "Sign up failed.")
		return
	}
	c.JSON(http.StatusCreated, u)
}

func (h *Handler) GetUser(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if u, err := h.UserService.GetUser(c.Request.Context(), id); err != nil {
			mw.AbortWithNotFoundError(c, "User", "id", c.Param("id"))
			return
		} else {
			c.JSON(http.StatusOK, u)
		}
	}
}

func (h *Handler) GetUsers(c *gin.Context) {

	ids, err := strings.ParseCSVToInt32(c.Query("ids"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid ids parameter")
		return
	}

	if len(ids) != 0 {
		if users, err := h.UserService.GetUsers(c.Request.Context(), ids); err != nil {
			mw.AbortWithInternalError(c, "Failed to get users")
			return
		} else {
			c.JSON(http.StatusOK, model.Page[model.User]{Total: len(users), Hits: users})
		}
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

	if users, total, err := h.UserService.GetUsersPage(c.Request.Context(), pattern, skip, limit); err != nil {
		mw.AbortWithInternalError(c, "Failed to get users")
	} else {
		c.JSON(http.StatusOK, model.Page[model.User]{Total: total, Hits: users})
	}
}

func (h *Handler) UpdateUser(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithInternalError(c, "Invalid id parameter")
		return
	} else {

		jsonData, err := io.ReadAll(c.Request.Body)
		if err != nil {
			mw.AbortWithBadRequestError(c, "Could not read request body")
			return
		}

		// Validate body
		if err := ValidateBody(jsonData, &model.UserAttributes{}); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}

		// memberids maps onto the user_group_member join table, not a users
		// column
		var body struct {
			MemberIDs []int32 `json:"memberids"`
		}
		if err := json.Unmarshal(jsonData, &body); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}

		// Set field values (membership is handled separately below)
		var fields map[string]interface{}
		if err := json.Unmarshal(jsonData, &fields); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}
		delete(fields, "memberids")
		if err := h.UserService.UpdateUser(c.Request.Context(), id, &fields); err != nil {
			if mw.AbortWithBadRequestIfReadOnlyField(c, err) {
				return
			}
			if errors.Is(err, service.ErrSignInNameTaken) {
				mw.Abort(c, mw.New(http.StatusConflict, "Conflict", "A user with this sign in name already exists"))
				return
			}
			mw.AbortWithInternalError(c, "Failed to update user")
			return
		}

		// Changing group membership is a super-user action; a user must not be
		// able to add themselves to groups via self-service patching.
		if body.MemberIDs != nil {
			user, exists := mw.AuthUserFromContext(c)
			if !exists || !user.SuperUser {
				mw.AbortWithForbiddenError(c, "Missing privileges")
				return
			}
			if err := h.UserGroupService.SetUserMemberships(c.Request.Context(), id, body.MemberIDs); err != nil {
				mw.AbortWithInternalError(c, "Failed to update user group memberships")
				return
			}
		}

		// Passwords are never updated via this route; self-service changes go
		// through PUT /auth/password.
		c.JSON(http.StatusOK, gin.H{"message": "User updated"})
	}
}

func (h *Handler) DeleteUser(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if err := h.UserService.DeleteUser(c.Request.Context(), id); err != nil {
			if mw.AbortWithNotFoundIfNoRows(c, err, "user", "id", c.Param("id")) {
				return
			}
			mw.AbortWithInternalError(c, "Failed to delete user")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
	}
}

func (h *Handler) GetUserImage(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if imageData, mimeType, err := h.UserService.GetUserImage(c.Request.Context(), id); err != nil {
			mw.AbortWithNotFoundError(c, "UserImage", "id", c.Param("id"))
			return
		} else {
			c.Data(http.StatusOK, mimeType, imageData)
		}
	}
}

func (h *Handler) CreateUserImage(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	// Request contains raw binary data
	err = h.UserService.CreateUserImage(c.Request.Context(), id, c.Request.Body)
	if err != nil {
		if errors.Is(err, service.ErrUnsupportedImageType) {
			mw.Abort(c, mw.New(http.StatusUnsupportedMediaType, "UnsupportedMediaType", err.Error()))
		} else if errors.Is(err, service.ErrImageTooLarge) {
			mw.Abort(c, mw.New(http.StatusRequestEntityTooLarge, "ImageTooLarge", err.Error()))
		} else {
			mw.AbortWithInternalError(c, "Failed to upload user image")
		}
		return
	}

	// Return a success response
	c.Status(http.StatusCreated)
}

func (h *Handler) DeleteUserImage(c *gin.Context) {

	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if err := h.UserService.DeleteUserImage(c.Request.Context(), id); err != nil {
			if mw.AbortWithNotFoundIfNoRows(c, err, "UserImage", "id", c.Param("id")) {
				return
			}
			mw.AbortWithInternalError(c, "Failed to delete user image")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "User image deleted"})
	}

}

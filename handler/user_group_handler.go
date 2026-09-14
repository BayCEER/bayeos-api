package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	mw "github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/strings"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetUserGroups(c *gin.Context) {
	ids, err := strings.ParseCSVToInt32(c.Query("ids"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid ids parameter")
		return
	}

	if groups, err := h.UserGroupService.GetUserGroups(c.Request.Context(), ids); err != nil {
		mw.AbortWithInternalError(c, "Failed to get user groups")
		return
	} else {
		c.JSON(http.StatusOK, groups)
	}
}

func (h *Handler) CreateUserGroup(c *gin.Context) {
	var cre model.UserGroupCreate
	if err := c.BindJSON(&cre); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}
	g := model.UserGroup{DisplayName: cre.DisplayName}
	if err := h.UserGroupService.CreateUserGroup(c.Request.Context(), &g); err != nil {
		mw.AbortWithInternalError(c, "Failed to create user group")
		return
	}
	c.JSON(http.StatusCreated, g)
}

func (h *Handler) GetUserGroup(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if g, err := h.UserGroupService.GetUserGroup(c.Request.Context(), id); err != nil {
			mw.AbortWithNotFoundError(c, "UserGroup", "id", c.Param("id"))
			return
		} else {
			c.JSON(http.StatusOK, g)
		}
	}
}

func (h *Handler) UpdateUserGroup(c *gin.Context) {
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
		if err := ValidateBody(jsonData, &model.UserGroupAttributes{}); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}

		// Set field values
		var fields map[string]interface{}
		if err := json.Unmarshal(jsonData, &fields); err != nil {
			mw.AbortWithBadRequestError(c, "Invalid request body")
			return
		}
		if err := h.UserGroupService.UpdateUserGroup(c.Request.Context(), id, &fields); err != nil {
			mw.AbortWithInternalError(c, "Failed to update user group")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "User group updated"})
	}
}

func (h *Handler) DeleteUserGroup(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if err := h.UserGroupService.DeleteUserGroup(c.Request.Context(), id); err != nil {
			if mw.AbortWithNotFoundIfNoRows(c, err, "UserGroup", "id", c.Param("id")) {
				return
			}
			mw.AbortWithInternalError(c, "Failed to delete user group")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "User group deleted"})
	}
}

func (h *Handler) GetUserGroupMembers(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if users, err := h.UserGroupService.GetUserGroupMembers(c.Request.Context(), id); err != nil {
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get user group members: %v", err))
			return
		} else {
			c.JSON(http.StatusOK, users)
		}
	}
}

func (h *Handler) AddUserGroupMembers(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}

	var req model.UserGroupMembersRequest
	if err := c.BindJSON(&req); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid request body")
		return
	}

	if err := h.UserGroupService.AddUserGroupMembers(c.Request.Context(), id, req.UserIDs); err != nil {
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to add user group members: %v", err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Members added"})
}

func (h *Handler) RemoveUserGroupMember(c *gin.Context) {
	id, err := strings.ParseStringToInt32(c.Param("id"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	}

	uid, err := strings.ParseStringToInt32(c.Param("uid"))
	if err != nil {
		mw.AbortWithBadRequestError(c, "Invalid member id parameter")
		return
	}

	if err := h.UserGroupService.RemoveUserGroupMember(c.Request.Context(), id, uid); err != nil {
		if mw.AbortWithNotFoundIfNoRows(c, err, "UserGroup member", "uid", c.Param("uid")) {
			return
		}
		mw.AbortWithInternalError(c, fmt.Sprintf("Failed to remove user group member: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Member removed"})
}

func (h *Handler) GetUserGroupPrivileges(c *gin.Context) {
	if id, err := strings.ParseStringToInt32(c.Param("id")); err != nil {
		mw.AbortWithBadRequestError(c, "Invalid id parameter")
		return
	} else {
		if privs, err := h.UserGroupService.GetUserGroupPrivileges(c.Request.Context(), id); err != nil {
			mw.AbortWithInternalError(c, fmt.Sprintf("Failed to get user group privileges: %v", err))
			return
		} else {
			c.JSON(http.StatusOK, privs)
		}
	}
}

package model

import "gopkg.in/guregu/null.v4"

type UserGroup struct {
	ID          int32     `db:"id" json:"id"`
	DisplayName string    `db:"display_name" json:"display_name"`
	SysGroup    bool      `db:"sys_group" json:"sys_group"`
	CreatedAt   null.Time `db:"created_at" json:"created_at"`
	// MemberIDs is populated by the service layer; there is no dedicated
	// database column on user_group (members live in user_group_member).
	MemberIDs []int32 `db:"-" json:"memberids,omitempty"`
}

type UserGroupAttributes struct {
	DisplayName string `db:"display_name" json:"display_name"`
}

type UserGroupCreate struct {
	DisplayName string `json:"display_name" binding:"required"`
}

type UserGroupMembersRequest struct {
	UserIDs []int32 `json:"user_ids" binding:"required"`
}

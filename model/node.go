package model

import (
	"github.com/google/uuid"
	"gopkg.in/guregu/null.v4"
)

const ROOT_ID = 1
const HOME_ID = 2

type Node struct {
	ID          int32     `db:"id" json:"id"`
	DisplayName string    `db:"display_name" json:"display_name"`
	Kind        string    `db:"kind" json:"kind"`
	ParentID    int32     `db:"parent_id" json:"parent_id"`
	Permission  uint8     `db:"permission" json:"permission"`
	NodePath    string    `db:"node_path" json:"node_path"`
	CreatedAt   null.Time `db:"created_at" json:"created_at"`
}

type NodePrivilege struct {
	ID         int32 `db:"id" json:"id"`
	NodeID     int32 `db:"node_id" json:"node_id"`
	GranteeID  int32 `db:"grantee_id" json:"grantee_id"`
	GrantorID  int32 `db:"grantor_id" json:"grantor_id"`
	Permission uint8 `db:"permission" json:"permission"`
}

type NodeToken struct {
	ID          int32     `db:"id" json:"id"`
	NodeID      int32     `db:"node_id" json:"node_id"`
	DisplayName string    `db:"display_name" json:"display_name"`
	Active      bool      `db:"active" json:"active"`
	Token       uuid.UUID `db:"token" json:"token"`
}

type NodeSearch struct {
	Total int32  `json:"total"`
	Hits  []Node `json:"hits"`
}

func NewNodePrivilege(nodeID int32, grantorID int32, granteeID int32) NodePrivilege {
	return NodePrivilege{NodeID: nodeID, GrantorID: grantorID, GranteeID: granteeID, Permission: FULL_PERMISSION}
}

func NewNodeToken(nodeID int32) NodeToken {
	uu, _ := uuid.NewRandom()
	return NodeToken{NodeID: nodeID, Token: uu}
}

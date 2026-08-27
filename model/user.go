package model

import "gopkg.in/guregu/null.v4"

type User struct {
	ID             int32     `db:"id" json:"id"`
	DisplayName    string    `db:"display_name" json:"display_name"`
	SysUser        bool      `db:"sys_user" json:"sys_user"`
	SignInName     string    `db:"sign_in_name" json:"sign_in_name"`
	EmailAddress   string    `db:"email_address" json:"email_address"`
	SuperUser      bool      `db:"super_user" json:"super_user"`
	HomeNodeID     int32     `db:"home_node_id" json:"home_node_id"`
	PasswordHash   string    `db:"password_hash,omitempty" json:"-"`
	LastSignInTime null.Time `db:"last_sign_in_time" json:"last_sign_in_time"`
}

type UserAttributes struct {
	DisplayName  string `db:"display_name" json:"display_name"`
	SignInName   string `db:"sign_in_name" json:"sign_in_name"`
	EmailAddress string `db:"email_address" json:"email_address"`
	SuperUser    bool   `db:"super_user" json:"super_user"`
}

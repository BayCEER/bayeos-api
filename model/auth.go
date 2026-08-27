package model

type SignInCredentials struct {
	Name     string `form:"name" json:"name" binding:"required"`
	Password string `form:"password" json:"password" binding:"required"`
}

type SignUpCredentials struct {
	SignInName   string `json:"sign_in_name" binding:"required"`
	DisplayName  string `json:"display_name" binding:"required"`
	EmailAddress string `json:"email_address" binding:"required"`
	Password     string `json:"password" binding:"required"`
}

package model

type IdBody struct {
	IDs []int32 `json:"id"`
}

type IdFormQuery struct {
	IDs []int32 `form:"id"`
}

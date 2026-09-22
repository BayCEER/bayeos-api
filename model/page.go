package model

// Page is a generic paginated response envelope matching the node search
// response shape ({ total, hits }).
type Page[T any] struct {
	Total int `json:"total"`
	Hits  []T `json:"hits"`
}

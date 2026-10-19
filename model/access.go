package model

import "context"

// NodeAccess authorizes node-scoped operations for one request subject. It is
// the single seam through which the service layer asks "may this subject act on
// nodeID?", regardless of whether the subject is an authenticated user or a
// node token.
type NodeAccess interface {
	// Permission returns the permission bits (READ/WRITE/GRANT) the subject
	// holds on nodeID. A subject without any access returns 0 and a nil error.
	Permission(ctx context.Context, nodeID int32) (uint8, error)
}

// UserScoped is an optional capability implemented by access subjects that act
// on behalf of a user. Only a user has a home node, so path patterns that start
// with "~" can only be resolved when the subject implements it.
type UserScoped interface {
	UserID() int32
}

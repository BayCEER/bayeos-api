package service

import (
	"context"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
)

// userAccess authorizes a request on behalf of an authenticated user. It
// implements model.UserScoped because only a user has a home node.
type userAccess struct {
	repo   repository.SqlRepo
	userID int32
	super  int8 // -1 unknown, 0 not a super user, 1 super user
}

func (a *userAccess) UserID() int32 { return a.userID }

// isSuper caches the super-user flag for the lifetime of the access subject so
// that listing many nodes does not repeat the lookup. A failed lookup is not
// cached and not treated as authoritative: Permission falls through to
// get_node_permission, which re-reads the flag itself.
func (a *userAccess) isSuper(ctx context.Context) bool {
	if a.super < 0 {
		super, err := a.repo.IsSuperUser(ctx, a.userID)
		if err != nil {
			return false
		}
		if super {
			a.super = 1
		} else {
			a.super = 0
		}
	}
	return a.super == 1
}

func (a *userAccess) Permission(ctx context.Context, nodeID int32) (uint8, error) {
	if a.isSuper(ctx) {
		return model.FULL_PERMISSION, nil
	}
	// get_node_permission already grants full permission to super users.
	return a.repo.GetNodePermission(ctx, nodeID, a.userID)
}

// tokenAccess authorizes a request authenticated by a node token. The token
// carries a fixed permission set that applies to its node and every descendant,
// and nowhere else.
type tokenAccess struct {
	repo repository.SqlRepo
	node int32
	perm uint8
}

func (a tokenAccess) Permission(ctx context.Context, nodeID int32) (uint8, error) {
	in, err := a.repo.IsNodeInSubtree(ctx, a.node, nodeID)
	if err != nil {
		return 0, err
	}
	if !in {
		return 0, nil
	}
	return a.perm, nil
}

// NodeAccessFactory builds a model.NodeAccess for the subject of a request. It
// is the only place that knows how an authenticated principal is turned into
// node permissions.
type NodeAccessFactory struct {
	repo repository.SqlRepo
}

func NewNodeAccessFactory(repo repository.SqlRepo) NodeAccessFactory {
	return NodeAccessFactory{repo: repo}
}

func (f NodeAccessFactory) ForUser(userID int32) model.NodeAccess {
	return &userAccess{repo: f.repo, userID: userID, super: -1}
}

func (f NodeAccessFactory) ForToken(nodeID int32, permission uint8) model.NodeAccess {
	return tokenAccess{repo: f.repo, node: nodeID, perm: permission}
}

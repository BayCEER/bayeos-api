package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/google/uuid"
)

type NodeService interface {
	GetNode(ctx context.Context, id int32) (model.Node, error)
	GetNodes(ctx context.Context, ids []int32) ([]model.Node, error)
	GetNodeChildren(ctx context.Context, id int32) ([]model.Node, error)
	GetNodeParents(ctx context.Context, id int32) ([]model.Node, error)
	CreateNode(ctx context.Context, node *model.Node) error
	UpdateNode(ctx context.Context, id int32, fields *map[string]interface{}) error
	DeleteNode(ctx context.Context, id int32) error
	FindNodeByToken(ctx context.Context, token uuid.UUID) (model.Node, error)
	FindNodesByPath(ctx context.Context, query string, skip int, limit int) (int32, []model.Node, error)

	GetNodeTokens(ctx context.Context, id int32) ([]model.NodeToken, error)
	GetNodePrivs(ctx context.Context, id int32) ([]model.NodePrivilege, error)
	HasPermission(ctx context.Context, id int32, perm uint8, userID int32) (bool, error)
}

type nodeService struct {
	sqlRepo repository.SqlRepo
}

func NewNodeService(sqlRepo repository.SqlRepo) NodeService {
	return &nodeService{
		sqlRepo: sqlRepo,
	}
}

func (s *nodeService) getUserReadableNodes(ctx context.Context, nodes []model.Node) ([]model.Node, error) {

	user, ok := ctx.Value("User").(model.User)
	if !ok {
		return []model.Node{}, errors.New("missing context information for user")
	}

	readableNodes := []model.Node{}
	for _, node := range nodes {
		if p, err := s.sqlRepo.GetNodePermission(node.ID, user.ID); err != nil {
			return []model.Node{}, errors.New("failed to get node permission")
		} else {
			if model.Readable(p) {
				node.Permission = p
				readableNodes = append(readableNodes, node)
			}
		}
	}
	return readableNodes, nil
}

func (s *nodeService) GetNode(ctx context.Context, id int32) (model.Node, error) {

	n, err := s.sqlRepo.FindNodeByID(id)
	if err != nil {
		return model.Node{}, fmt.Errorf("node with id %d not found: %w", id, err)
	}

	user, ok := ctx.Value("User").(model.User)
	if !ok {
		return model.Node{}, errors.New("missing context information for user")
	}

	if p, err := s.sqlRepo.GetNodePermission(n.ID, user.ID); err != nil {
		return model.Node{}, errors.New("failed to get node permission")
	} else {
		n.Permission = p
		return n, nil
	}
}

func (s *nodeService) GetNodes(ctx context.Context, ids []int32) ([]model.Node, error) {
	var nodes []model.Node
	var err error

	if len(ids) == 0 {
		nodes, err = s.sqlRepo.FindNodesByIDs([]int32{model.ROOT_ID})
	} else {
		nodes, err = s.sqlRepo.FindNodesByIDs(ids)
	}
	if err != nil {
		return []model.Node{}, err
	}
	return s.getUserReadableNodes(ctx, nodes)
}

func (s *nodeService) GetNodeChildren(ctx context.Context, id int32) ([]model.Node, error) {
	chs, err := s.sqlRepo.FindNodeChildrenByID(id)
	if err != nil {
		return []model.Node{}, fmt.Errorf("children of node with id %d not found: %w", id, err)
	}
	return s.getUserReadableNodes(ctx, chs)
}

func (s *nodeService) GetNodeParents(ctx context.Context, id int32) ([]model.Node, error) {

	parents, err := s.sqlRepo.FindNodeParentsByID(id)
	if err != nil {
		return []model.Node{}, fmt.Errorf("parents of node with id %d not found: %w", id, err)
	}

	return s.getUserReadableNodes(ctx, parents)
}

func (s *nodeService) GetNodeTokens(ctx context.Context, id int32) ([]model.NodeToken, error) {
	tokens, err := s.sqlRepo.FindNodeTokensByNodeID(id)
	if err != nil {
		return []model.NodeToken{}, fmt.Errorf("tokens of node with id %d not found: %w", id, err)
	}
	return tokens, nil
}

func (s *nodeService) GetNodePrivs(ctx context.Context, id int32) ([]model.NodePrivilege, error) {
	privs, err := s.sqlRepo.FindNodePrivsByNodeID(id)
	if err != nil {
		return []model.NodePrivilege{}, fmt.Errorf("privileges of node with id %d not found: %w", id, err)
	}
	return privs, nil
}

func (s *nodeService) FindNodesByPath(ctx context.Context, query string, skip int, limit int) (int32, []model.Node, error) {
	nodes := []model.Node{}

	user, ok := ctx.Value("User").(model.User)
	if !ok {
		return 0, nodes, errors.New("missing context information for user")
	}

	hits, err := s.sqlRepo.FindNodesByPath(query)
	if err != nil {
		return 0, hits, errors.New("failed to find nodes")
	}
	var total int32
	for i, hit := range hits {
		if p, err := s.sqlRepo.GetNodePermission(hit.ID, user.ID); err != nil {
			return 0, nodes, errors.New("failed to get node permission")
		} else {
			if model.Readable(p) {
				hit.Permission = p
				total = total + 1
				// Skip and limit
				if (skip <= i) && (i < skip+limit) {
					nodes = append(nodes, hit)
				}
			}
		}
	}
	return total, nodes, nil
}

func (s *nodeService) CreateNode(ctx context.Context, node *model.Node) error {
	if err := s.sqlRepo.CreateNode(node); err != nil {
		return errors.New("failed to create node")
	}

	user, ok := ctx.Value("User").(model.User)
	if !ok {
		return errors.New("missing context information for user")
	}

	if p, err := s.sqlRepo.GetNodePermission(node.ID, user.ID); err != nil {
		return errors.New("failed to get node permission")
	} else {
		node.Permission = p
	}

	return nil
}

func (s *nodeService) DeleteNode(ctx context.Context, id int32) error {

	if err := s.sqlRepo.DeleteNodeByID(id); err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}
	return nil
}

func (s *nodeService) UpdateNode(ctx context.Context, id int32, fields *map[string]interface{}) error {

	if err := s.sqlRepo.Do(context.Background(), func(repo repository.SqlRepo) error {
		for k, v := range *fields {
			if err := s.sqlRepo.UpdateNodeFieldByID(id, k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return errors.New("failed to update node")
	}
	return nil
}

func (s *nodeService) FindNodeByToken(ctx context.Context, token uuid.UUID) (model.Node, error) {

	var node model.Node
	node, err := s.sqlRepo.FindNodeByToken(token)
	if err != nil {
		return node, errors.New("node not found")
	}

	user, ok := ctx.Value("User").(model.User)
	if !ok {
		return model.Node{}, errors.New("missing context information for user")
	}

	if p, err := s.sqlRepo.GetNodePermission(node.ID, user.ID); err != nil {
		return node, errors.New("failed to get node permission")
	} else {
		node.Permission = p
		return node, nil
	}

}

// Implements middleware.NodeAuthorizer
func (s *nodeService) HasPermission(ctx context.Context, id int32, perm uint8, userID int32) (bool, error) {
	granted, err := s.sqlRepo.GetNodePermission(id, userID)
	if err != nil {
		return false, errors.New("failed to get node permission")
	}
	return model.HasPermission(granted, perm), nil

}

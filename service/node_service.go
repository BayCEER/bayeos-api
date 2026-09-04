package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/google/uuid"
)

type NodeService interface {
	GetNode(ctx context.Context, id int32, userID int32) (model.Node, error)
	GetNodes(ctx context.Context, ids []int32, userID int32) ([]model.Node, error)
	GetNodeChildren(ctx context.Context, id int32, userID int32) ([]model.Node, error)
	GetNodeParents(ctx context.Context, id int32, userID int32) ([]model.Node, error)
	CreateNode(ctx context.Context, node *model.Node, userID int32) error
	CreateNodeByPath(ctx context.Context, node *model.Node, baseNodeID int32, path string, userID int32) error
	UpdateNode(ctx context.Context, id int32, fields *map[string]interface{}) error
	UpdateNodeMetadata(ctx context.Context, id int32, metadata map[string]any) error
	DeleteNode(ctx context.Context, id int32) error
	FindNodeDescendantsByID(ctx context.Context, id int32) ([]int32, error)
	DeleteNodesByIDs(ctx context.Context, ids []int32) error
	FindNodeByToken(ctx context.Context, token uuid.UUID) (model.Node, error)
	FindNodesByAntPath(ctx context.Context, pattern string, kind string, skip int, limit int, userID int32) (int32, []model.Node, error)

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

func (s *nodeService) getUserReadableNodes(ctx context.Context, nodes []model.Node, userID int32) ([]model.Node, error) {

	readableNodes := []model.Node{}
	for _, node := range nodes {
		if p, err := s.sqlRepo.GetNodePermission(ctx, node.ID, userID); err != nil {
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

func (s *nodeService) GetNode(ctx context.Context, id int32, userID int32) (model.Node, error) {

	n, err := s.sqlRepo.FindNodeByID(ctx, id)
	if err != nil {
		return model.Node{}, fmt.Errorf("node with id %d not found: %w", id, err)
	}

	if p, err := s.sqlRepo.GetNodePermission(ctx, n.ID, userID); err != nil {
		return model.Node{}, errors.New("failed to get node permission")
	} else {
		n.Permission = p
		return n, nil
	}
}

func (s *nodeService) GetNodes(ctx context.Context, ids []int32, userID int32) ([]model.Node, error) {
	var nodes []model.Node
	var err error

	if len(ids) == 0 {
		nodes, err = s.sqlRepo.FindNodesByIDs(ctx, []int32{model.ROOT_ID})
	} else {
		nodes, err = s.sqlRepo.FindNodesByIDs(ctx, ids)
	}
	if err != nil {
		return []model.Node{}, err
	}
	return s.getUserReadableNodes(ctx, nodes, userID)
}

func (s *nodeService) GetNodeChildren(ctx context.Context, id int32, userID int32) ([]model.Node, error) {
	chs, err := s.sqlRepo.FindNodeChildrenByID(ctx, id)
	if err != nil {
		return []model.Node{}, fmt.Errorf("children of node with id %d not found: %w", id, err)
	}
	return s.getUserReadableNodes(ctx, chs, userID)
}

func (s *nodeService) GetNodeParents(ctx context.Context, id int32, userID int32) ([]model.Node, error) {

	parents, err := s.sqlRepo.FindNodeParentsByID(ctx, id)
	if err != nil {
		return []model.Node{}, fmt.Errorf("parents of node with id %d not found: %w", id, err)
	}

	return s.getUserReadableNodes(ctx, parents, userID)
}

func (s *nodeService) GetNodeTokens(ctx context.Context, id int32) ([]model.NodeToken, error) {
	tokens, err := s.sqlRepo.FindNodeTokensByNodeID(ctx, id)
	if err != nil {
		return []model.NodeToken{}, fmt.Errorf("tokens of node with id %d not found: %w", id, err)
	}
	return tokens, nil
}

func (s *nodeService) GetNodePrivs(ctx context.Context, id int32) ([]model.NodePrivilege, error) {
	privs, err := s.sqlRepo.FindNodePrivsByNodeID(ctx, id)
	if err != nil {
		return []model.NodePrivilege{}, fmt.Errorf("privileges of node with id %d not found: %w", id, err)
	}
	return privs, nil
}

func (s *nodeService) FindNodesByAntPath(ctx context.Context, pattern string, kind string, skip int, limit int, userID int32) (int32, []model.Node, error) {
	regex, err := antToRegex(pattern)
	if err != nil {
		return 0, nil, err
	}

	hits, err := s.sqlRepo.FindNodesByAntPath(ctx, regex, kind)
	if err != nil {
		return 0, nil, errors.New("failed to find nodes")
	}

	nodes := []model.Node{}
	var total int32
	for i, hit := range hits {
		if p, err := s.sqlRepo.GetNodePermission(ctx, hit.ID, userID); err != nil {
			return 0, nil, errors.New("failed to get node permission")
		} else {
			if model.Readable(p) {
				hit.Permission = p
				total++
				if skip <= i && i < skip+limit {
					nodes = append(nodes, hit)
				}
			}
		}
	}
	return total, nodes, nil
}

func (s *nodeService) CreateNode(ctx context.Context, node *model.Node, userID int32) error {
	if err := s.sqlRepo.CreateNode(ctx, node); err != nil {
		return errors.New("failed to create node")
	}

	if p, err := s.sqlRepo.GetNodePermission(ctx, node.ID, userID); err != nil {
		return errors.New("failed to get node permission")
	} else {
		node.Permission = p
	}

	return nil
}

// CreateNodeByPath resolves a slash-delimited path relative to baseNodeID,
// creates intermediate Folder nodes as needed, and upserts the leaf node with
// the given kind. The path must be relative (no leading "/" or "~").
func (s *nodeService) CreateNodeByPath(ctx context.Context, node *model.Node, baseNodeID int32, path string, userID int32) error {
	if path == "" {
		return errors.New("path must not be empty")
	}
	if path[0] == '/' || path[0] == '~' {
		return errors.New("path must not contain prefix; resolve base node before calling")
	}
	result, err := s.sqlRepo.FindOrCreateNodesByPath(ctx, baseNodeID, path, node.Kind)
	if err != nil {
		if errors.Is(err, repository.ErrKindConflict) {
			return fmt.Errorf("kind conflict at path %q: %w", path, err)
		}
		return fmt.Errorf("failed to resolve path %q: %w", path, err)
	}
	*node = result
	if p, err := s.sqlRepo.GetNodePermission(ctx, node.ID, userID); err != nil {
		return errors.New("failed to get node permission")
	} else {
		node.Permission = p
	}
	return nil
}

func (s *nodeService) DeleteNode(ctx context.Context, id int32) error {

	if err := s.sqlRepo.DeleteNodeByID(ctx, id); err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}
	return nil
}

func (s *nodeService) FindNodeDescendantsByID(ctx context.Context, id int32) ([]int32, error) {
	ids, err := s.sqlRepo.FindNodeDescendantsByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find descendants of node %d: %w", id, err)
	}
	return ids, nil
}

func (s *nodeService) DeleteNodesByIDs(ctx context.Context, ids []int32) error {
	if err := s.sqlRepo.DeleteNodesByIDs(ctx, ids); err != nil {
		return fmt.Errorf("failed to delete nodes: %w", err)
	}
	return nil
}

func (s *nodeService) UpdateNode(ctx context.Context, id int32, fields *map[string]interface{}) error {

	if err := s.sqlRepo.Do(ctx, func(repo repository.SqlRepo) error {
		for k, v := range *fields {
			if k == "metadata" {
				var md model.Metadata
				switch t := v.(type) {
				case map[string]any:
					var err error
					md, err = model.MetadataFromMap(t)
					if err != nil {
						return err
					}
				case nil:
					// clearing not supported here; the DB default keeps it {}
					continue
				default:
					data, err := json.Marshal(v)
					if err != nil {
						return err
					}
					md = model.NewMetadata(data)
				}
				if err := s.sqlRepo.MergeNodeMetadata(ctx, id, md); err != nil {
					return err
				}
				continue
			}
			if err := s.sqlRepo.UpdateNodeFieldByID(ctx, id, k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return errors.New("failed to update node")
	}
	return nil
}

func (s *nodeService) UpdateNodeMetadata(ctx context.Context, id int32, metadata map[string]any) error {
	md, err := model.MetadataFromMap(metadata)
	if err != nil {
		return err
	}
	return s.sqlRepo.MergeNodeMetadata(ctx, id, md)
}

func (s *nodeService) FindNodeByToken(ctx context.Context, token uuid.UUID) (model.Node, error) {

	var node model.Node
	node, err := s.sqlRepo.FindNodeByToken(ctx, token)
	if err != nil {
		return node, errors.New("node not found")
	}
	return node, nil
}

// Implements middleware.NodeAuthorizer
func (s *nodeService) HasPermission(ctx context.Context, id int32, perm uint8, userID int32) (bool, error) {
	granted, err := s.sqlRepo.GetNodePermission(ctx, id, userID)
	if err != nil {
		return false, errors.New("failed to get node permission")
	}
	return model.HasPermission(granted, perm), nil

}

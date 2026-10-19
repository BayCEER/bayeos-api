package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"go.opentelemetry.io/otel/attribute"
)

type NodeService interface {
	GetNode(ctx context.Context, id int32, access model.NodeAccess) (model.Node, error)
	GetNodes(ctx context.Context, ids []int32, access model.NodeAccess) ([]model.Node, error)
	GetNodeChildren(ctx context.Context, id int32, access model.NodeAccess) ([]model.Node, error)
	GetNodeParents(ctx context.Context, id int32, access model.NodeAccess) ([]model.Node, error)
	CreateNode(ctx context.Context, node *model.Node, access model.NodeAccess) error
	CreateNodeByPath(ctx context.Context, node *model.Node, baseNodeID int32, path string, access model.NodeAccess) error
	UpdateNode(ctx context.Context, id int32, fields *map[string]interface{}) error
	UpdateNodeMetadata(ctx context.Context, id int32, metadata map[string]any) error
	DeleteNode(ctx context.Context, id int32) error
	FindNodeDescendantsByID(ctx context.Context, id int32) ([]int32, error)
	DeleteNodesByIDs(ctx context.Context, ids []int32) error

	FindNodesByAntPath(ctx context.Context, pattern string, kind string, skip int, limit int, access model.NodeAccess) (int32, []model.Node, error)

	GetNodeTokens(ctx context.Context, id int32) ([]model.NodeToken, error)
	GetNodePrivs(ctx context.Context, id int32) ([]model.NodePrivilege, error)
}

type nodeService struct {
	sqlRepo repository.SqlRepo
}

func NewNodeService(sqlRepo repository.SqlRepo) NodeService {
	return &nodeService{
		sqlRepo: sqlRepo,
	}
}

func paginateHits(hits []model.Node, skip int, limit int) []model.Node {
	start := skip
	if start < 0 {
		start = 0
	}
	end := skip + limit
	if end > len(hits) {
		end = len(hits)
	}
	if end <= start {
		return []model.Node{}
	}
	return hits[start:end]
}

// readableNodes drops the nodes the subject may not read and annotates the rest
// with the permission the subject holds on them. Permission is asked per node
// through the access subject, so a user and a node token resolve it differently
// without this function knowing which it is.
func (s *nodeService) readableNodes(ctx context.Context, nodes []model.Node, access model.NodeAccess) ([]model.Node, error) {
	readable := []model.Node{}
	for _, node := range nodes {
		p, err := access.Permission(ctx, node.ID)
		if err != nil {
			return []model.Node{}, fmt.Errorf("failed to get node permission: %w", err)
		}
		if model.Readable(p) {
			node.Permission = p
			readable = append(readable, node)
		}
	}
	return readable, nil
}

func (s *nodeService) GetNode(ctx context.Context, id int32, access model.NodeAccess) (n model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.get")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)
	setAccess(span, access)

	n, err = s.sqlRepo.FindNodeByID(ctx, id)
	if err != nil {
		return model.Node{}, fmt.Errorf("node with id %d not found: %w", id, err)
	}

	p, err := access.Permission(ctx, n.ID)
	if err != nil {
		return model.Node{}, fmt.Errorf("failed to get node permission: %w", err)
	}
	n.Permission = p
	return n, nil
}

func (s *nodeService) GetNodes(ctx context.Context, ids []int32, access model.NodeAccess) (nodes []model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.list")
	defer func() { endServiceSpan(span, err) }()
	setAccess(span, access)

	if len(ids) == 0 {
		nodes, err = s.sqlRepo.FindNodesByIDs(ctx, []int32{model.ROOT_ID})
	} else {
		nodes, err = s.sqlRepo.FindNodesByIDs(ctx, ids)
	}
	if err != nil {
		return []model.Node{}, err
	}
	return s.readableNodes(ctx, nodes, access)
}

func (s *nodeService) GetNodeChildren(ctx context.Context, id int32, access model.NodeAccess) (chs []model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.children")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)
	setAccess(span, access)

	chs, err = s.sqlRepo.FindNodeChildrenByID(ctx, id)
	if err != nil {
		return []model.Node{}, fmt.Errorf("children of node with id %d not found: %w", id, err)
	}
	return s.readableNodes(ctx, chs, access)
}

func (s *nodeService) GetNodeParents(ctx context.Context, id int32, access model.NodeAccess) (parents []model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.parents")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)
	setAccess(span, access)

	parents, err = s.sqlRepo.FindNodeParentsByID(ctx, id)
	if err != nil {
		return []model.Node{}, fmt.Errorf("parents of node with id %d not found: %w", id, err)
	}

	return s.readableNodes(ctx, parents, access)
}

func (s *nodeService) GetNodeTokens(ctx context.Context, id int32) (tokens []model.NodeToken, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.tokens")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)

	tokens, err = s.sqlRepo.FindNodeTokensByNodeID(ctx, id)
	if err != nil {
		return []model.NodeToken{}, fmt.Errorf("tokens of node with id %d not found: %w", id, err)
	}
	return tokens, nil
}

func (s *nodeService) GetNodePrivs(ctx context.Context, id int32) (privs []model.NodePrivilege, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.privileges")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)

	privs, err = s.sqlRepo.FindNodePrivsByNodeID(ctx, id)
	if err != nil {
		return []model.NodePrivilege{}, fmt.Errorf("privileges of node with id %d not found: %w", id, err)
	}
	return privs, nil
}

// homePath resolves the node_path of a user's home node: the node a leading "~"
// in a path stands for.
func (s *nodeService) homePath(ctx context.Context, userID int32) (string, error) {
	user, err := s.sqlRepo.FindUserByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to find user with id %d: %w", userID, err)
	}
	home, err := s.sqlRepo.FindNodeByID(ctx, user.HomeNodeID)
	if err != nil {
		return "", fmt.Errorf("failed to find home node with id %d: %w", user.HomeNodeID, err)
	}
	return home.NodePath, nil
}

func (s *nodeService) FindNodesByAntPath(ctx context.Context, pattern string, kind string, skip int, limit int, access model.NodeAccess) (total int32, nodes []model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.search_by_ant_path")
	defer func() { endServiceSpan(span, err) }()
	setAccess(span, access)
	span.SetAttributes(attribute.String("bayeos.pattern", pattern))

	// A leading "~" is the caller's home node, so the pattern only becomes an
	// Ant pattern once that prefix is a path. Only a user has a home node, so a
	// node-token subject cannot use it.
	if strings.HasPrefix(pattern, "~") {
		scoped, ok := access.(model.UserScoped)
		if !ok {
			return 0, nil, errors.New("home path is not available for node tokens")
		}
		var homePath string
		homePath, err = s.homePath(ctx, scoped.UserID())
		if err != nil {
			return 0, nil, err
		}
		pattern = expandHome(pattern, homePath)
		span.SetAttributes(attribute.String("bayeos.expanded_pattern", pattern))
	}

	regex, err := antToRegex(pattern)
	if err != nil {
		return 0, nil, err
	}

	hits, err := s.sqlRepo.FindNodesByAntPath(ctx, regex, kind)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to find nodes: %w", err)
	}

	nodes = []model.Node{}
	nodes, err = s.readableNodes(ctx, hits, access)
	if err != nil {
		return 0, nil, err
	}
	return int32(len(nodes)), paginateHits(nodes, skip, limit), nil
}

func (s *nodeService) CreateNode(ctx context.Context, node *model.Node, access model.NodeAccess) (err error) {
	ctx, span := withServiceSpan(ctx, "nodes.create")
	defer func() { endServiceSpan(span, err) }()
	setAccess(span, access)

	if err := s.sqlRepo.CreateNode(ctx, node); err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}
	setNodeID(span, node.ID)

	p, err := access.Permission(ctx, node.ID)
	if err != nil {
		return fmt.Errorf("failed to get node permission: %w", err)
	}
	node.Permission = p

	return nil
}

// CreateNodeByPath resolves a slash-delimited path relative to baseNodeID,
// creates intermediate Folder nodes as needed, and upserts the leaf node with
// the given kind. The path must be relative (no leading "/" or "~").
func (s *nodeService) CreateNodeByPath(ctx context.Context, node *model.Node, baseNodeID int32, path string, access model.NodeAccess) (err error) {
	ctx, span := withServiceSpan(ctx, "nodes.create_by_path")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, baseNodeID)
	setAccess(span, access)
	span.SetAttributes(attribute.String("bayeos.path", path))

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
	p, err := access.Permission(ctx, node.ID)
	if err != nil {
		return fmt.Errorf("failed to get node permission: %w", err)
	}
	node.Permission = p
	return nil
}

func (s *nodeService) DeleteNode(ctx context.Context, id int32) (err error) {
	ctx, span := withServiceSpan(ctx, "nodes.delete")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)

	if err := s.sqlRepo.DeleteNodeByID(ctx, id); err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}
	return nil
}

func (s *nodeService) FindNodeDescendantsByID(ctx context.Context, id int32) (ids []int32, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.descendants")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)

	ids, err = s.sqlRepo.FindNodeDescendantsByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find descendants of node %d: %w", id, err)
	}
	return ids, nil
}

func (s *nodeService) DeleteNodesByIDs(ctx context.Context, ids []int32) (err error) {
	ctx, span := withServiceSpan(ctx, "nodes.delete_many")
	defer func() { endServiceSpan(span, err) }()

	if err := s.sqlRepo.DeleteNodesByIDs(ctx, ids); err != nil {
		return fmt.Errorf("failed to delete nodes: %w", err)
	}
	return nil
}

func (s *nodeService) UpdateNode(ctx context.Context, id int32, fields *map[string]interface{}) (err error) {
	ctx, span := withServiceSpan(ctx, "nodes.update")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)

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
		return fmt.Errorf("failed to update node: %w", err)
	}
	return nil
}

func (s *nodeService) UpdateNodeMetadata(ctx context.Context, id int32, metadata map[string]any) (err error) {
	ctx, span := withServiceSpan(ctx, "nodes.update_metadata")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)

	md, err := model.MetadataFromMap(metadata)
	if err != nil {
		return err
	}
	return s.sqlRepo.MergeNodeMetadata(ctx, id, md)
}



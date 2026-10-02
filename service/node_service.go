package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
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

// isSuperUser reports whether the caller holds the super_user flag.
//
// get_node_permission grants FULL_PERMISSION to a super user without consulting
// node_privilege, so a super user's result never depends on the node. Callers
// that resolve a permission for many nodes, or that resolve it only to decorate
// a response, can ask here once and skip those per-node lookups entirely.
// paginateHits stamps full permission on every hit and returns the requested
// window, for the case where the caller is known to hold full permission on all
// of them. It reports no total, because the caller already knows every hit is
// visible and so can take len(hits) as the total itself.
func paginateHits(hits []model.Node, skip int, limit int) []model.Node {
	for i := range hits {
		hits[i].Permission = model.FULL_PERMISSION
	}
	// The window is the same set of indices the per-node loop selects: from
	// max(0, skip) up to skip+limit, clipped to the slice. Expressing it this
	// way keeps the odd inputs behaving as they always did, so a negative skip
	// selects from the start and a non-positive limit selects nothing.
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

func (s *nodeService) isSuperUser(ctx context.Context, userID int32) bool {
	super, err := s.sqlRepo.IsSuperUser(ctx, userID)
	if err != nil {
		// A failed lookup must not be read as "not a super user": that would
		// silently downgrade the caller's access. Fall through to the per-node
		// path, which re-reads the flag itself.
		return false
	}
	return super
}

func (s *nodeService) getUserReadableNodes(ctx context.Context, nodes []model.Node, userID int32) ([]model.Node, error) {

	// A super user can read every node, so the per-node permission lookups are
	// pure overhead: N+1 queries that can only ever return full permission.
	if s.isSuperUser(ctx, userID) {
		for i := range nodes {
			nodes[i].Permission = model.FULL_PERMISSION
		}
		return nodes, nil
	}

	readableNodes := []model.Node{}
	for _, node := range nodes {
		if p, err := s.sqlRepo.GetNodePermission(ctx, node.ID, userID); err != nil {
			return []model.Node{}, fmt.Errorf("failed to get node permission: %w", err)
		} else {
			if model.Readable(p) {
				node.Permission = p
				readableNodes = append(readableNodes, node)
			}
		}
	}
	return readableNodes, nil
}

func (s *nodeService) GetNode(ctx context.Context, id int32, userID int32) (n model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.get")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)
	setUserID(span, userID)

	n, err = s.sqlRepo.FindNodeByID(ctx, id)
	if err != nil {
		return model.Node{}, fmt.Errorf("node with id %d not found: %w", id, err)
	}

	if s.isSuperUser(ctx, userID) {
		n.Permission = model.FULL_PERMISSION
		return n, nil
	}

	if p, err := s.sqlRepo.GetNodePermission(ctx, n.ID, userID); err != nil {
		return model.Node{}, fmt.Errorf("failed to get node permission: %w", err)
	} else {
		n.Permission = p
		return n, nil
	}
}

func (s *nodeService) GetNodes(ctx context.Context, ids []int32, userID int32) (nodes []model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.list")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, userID)

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

func (s *nodeService) GetNodeChildren(ctx context.Context, id int32, userID int32) (chs []model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.children")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)
	setUserID(span, userID)

	chs, err = s.sqlRepo.FindNodeChildrenByID(ctx, id)
	if err != nil {
		return []model.Node{}, fmt.Errorf("children of node with id %d not found: %w", id, err)
	}
	return s.getUserReadableNodes(ctx, chs, userID)
}

func (s *nodeService) GetNodeParents(ctx context.Context, id int32, userID int32) (parents []model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.parents")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)
	setUserID(span, userID)

	parents, err = s.sqlRepo.FindNodeParentsByID(ctx, id)
	if err != nil {
		return []model.Node{}, fmt.Errorf("parents of node with id %d not found: %w", id, err)
	}

	return s.getUserReadableNodes(ctx, parents, userID)
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

func (s *nodeService) FindNodesByAntPath(ctx context.Context, pattern string, kind string, skip int, limit int, userID int32) (total int32, nodes []model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.findByAntPath")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, userID)
	span.SetAttributes(attribute.String("bayeos.search_pattern", pattern))

	regex, err := antToRegex(pattern)
	if err != nil {
		return 0, nil, err
	}

	hits, err := s.sqlRepo.FindNodesByAntPath(ctx, regex, kind)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to find nodes: %w", err)
	}

	// A super user sees every hit, so total is known without the per-node walk.
	if s.isSuperUser(ctx, userID) {
		return int32(len(hits)), paginateHits(hits, skip, limit), nil
	}

	nodes = []model.Node{}
	for i, hit := range hits {
		if p, err := s.sqlRepo.GetNodePermission(ctx, hit.ID, userID); err != nil {
			return 0, nil, fmt.Errorf("failed to get node permission: %w", err)
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

func (s *nodeService) CreateNode(ctx context.Context, node *model.Node, userID int32) (err error) {
	ctx, span := withServiceSpan(ctx, "nodes.create")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, userID)

	if err := s.sqlRepo.CreateNode(ctx, node); err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}
	setNodeID(span, node.ID)

	if p, err := s.sqlRepo.GetNodePermission(ctx, node.ID, userID); err != nil {
		return fmt.Errorf("failed to get node permission: %w", err)
	} else {
		node.Permission = p
	}

	return nil
}

// CreateNodeByPath resolves a slash-delimited path relative to baseNodeID,
// creates intermediate Folder nodes as needed, and upserts the leaf node with
// the given kind. The path must be relative (no leading "/" or "~").
func (s *nodeService) CreateNodeByPath(ctx context.Context, node *model.Node, baseNodeID int32, path string, userID int32) (err error) {
	ctx, span := withServiceSpan(ctx, "nodes.create_by_path")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, baseNodeID)
	setUserID(span, userID)
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
	if p, err := s.sqlRepo.GetNodePermission(ctx, node.ID, userID); err != nil {
		return fmt.Errorf("failed to get node permission: %w", err)
	} else {
		node.Permission = p
	}
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

func (s *nodeService) FindNodeByToken(ctx context.Context, token uuid.UUID) (node model.Node, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.find_by_token")
	defer func() { endServiceSpan(span, err) }()

	node, err = s.sqlRepo.FindNodeByToken(ctx, token)
	if err != nil {
		return node, fmt.Errorf("node not found: %w", err)
	}
	return node, nil
}

// Implements middleware.NodeAuthorizer
func (s *nodeService) HasPermission(ctx context.Context, id int32, perm uint8, userID int32) (allowed bool, err error) {
	ctx, span := withServiceSpan(ctx, "nodes.has_permission")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)
	setUserID(span, userID)

	// A super user is granted full permission on every node, so a request
	// holding it cannot fail on the permission being asked for.
	if s.isSuperUser(ctx, userID) {
		return true, nil
	}

	granted, err := s.sqlRepo.GetNodePermission(ctx, id, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get node permission: %w", err)
	}
	return model.HasPermission(granted, perm), nil
}

package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/google/uuid"
)

func (r *SqlRepo) FindNodeByID(ctx context.Context, ID int32) (model.Node, error) {
	var d model.Node
	stmt := `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id,  (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata FROM node WHERE id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &d, stmt, ID)
	endQuerySpan(span, err)
	return d, err
}

func (r *SqlRepo) FindNodesByIDs(ctx context.Context, ids []int32) ([]model.Node, error) {
	n := []model.Node{}
	stmt := `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id,  (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata FROM node WHERE id = ANY($1::int[])`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &n, stmt, ids)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (r *SqlRepo) FindNodeChildrenByID(ctx context.Context, ID int32) ([]model.Node, error) {
	n := []model.Node{}
	stmt := `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id,  (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata FROM node WHERE parent_id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &n, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return n, err
	}
	return n, nil
}

// FindNodeDescendantsByID returns the ID of the given node together with the
// IDs of all its descendants (children, grandchildren, …) using a recursive
// CTE that walks downward via parent_id. The node itself is included so the
// caller can clean up data for every node in the subtree.
func (r *SqlRepo) FindNodeDescendantsByID(ctx context.Context, ID int32) ([]int32, error) {
	ids := []int32{}
	stmt := `WITH RECURSIVE descendants AS (
		SELECT id FROM node WHERE id = $1
		UNION ALL
		SELECT n.id FROM node n JOIN descendants d ON n.parent_id = d.id
	) SELECT id FROM descendants`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &ids, stmt, ID)
	endQuerySpan(span, err)
	return ids, err
}

// DeleteNodesByIDs deletes all nodes whose id is in the given slice. The caller
// is responsible for providing the complete set of IDs to remove (including any
// descendants); the database ON DELETE CASCADE is a safety net, not the primary
// mechanism.
func (r *SqlRepo) DeleteNodesByIDs(ctx context.Context, ids []int32) error {
	if len(ids) == 0 {
		return nil
	}
	stmt := `DELETE FROM node WHERE id = ANY($1)`
	c, span := withQuerySpan(ctx, stmt)
	_, err := r.trans.ExecContext(c, stmt, ids)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) FindNodeParentsByID(ctx context.Context, ID int32) ([]model.Node, error) {
	n := []model.Node{}
	stmt := `with recursive wf as (
	SELECT id, display_name, kind, parent_id,  (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata, 0 as level from node where id = $1
    union all
    select n.id, n.display_name, n.kind, n.parent_id, (n.path).path as node_path, n.created_at, coalesce(n.metadata, '{}') as metadata, wf.level + 1 as level FROM wf JOIN node n on n.id = wf.parent_id
) 	SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, node_path, created_at, metadata from wf order by level desc `
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &n, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) FindNodeByIDAndKindAndName(ctx context.Context, ID int32, kind string, displayName string) (model.Node, error) {
	n := model.Node{}
	stmt := `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata 
	FROM node WHERE parent_id=$1 and kind=$2 and display_name=$3`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &n, stmt, ID, kind, displayName)
	endQuerySpan(span, err)
	if err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) CreateNode(ctx context.Context, n *model.Node) error {
	stmt := `INSERT INTO node (display_name,kind,parent_id,metadata) VALUES ($1,$2,$3,coalesce($4::jsonb,'{}')) RETURNING id,display_name,kind,coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, n, stmt,
		n.DisplayName, n.Kind, n.ParentID, n.Metadata)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) UpdateNode(ctx context.Context, n *model.Node) error {
	stmt := `UPDATE node set display_name=$1,kind=$2, parent_id=$3, metadata=coalesce($4::jsonb,'{}') where id=$5 RETURNING id,display_name,kind,coalesce(parent_id,0) as parent_id, (path).path as node_path, coalesce(metadata, '{}') as metadata`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, n, stmt,
		n.DisplayName, n.Kind, n.ParentID, n.Metadata, n.ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) DeleteNodeByID(ctx context.Context, ID int32) error {
	stmt := `DELETE FROM node where id = $1`
	c, span := withQuerySpan(ctx, stmt)
	err := execDeleteContext(c, r.trans, stmt, ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) FindNodesByAntPath(ctx context.Context, regex string, kind string) ([]model.Node, error) {
	n := []model.Node{}
	var stmt string
	var err error
	c, span := withQuerySpan(ctx, "FindNodesByAntPath")
	if kind == "" {
		stmt = `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata FROM node WHERE (path).path ~ $1 AND parent_id IS NOT NULL ORDER BY (path).path`
		err = r.trans.SelectContext(c, &n, stmt, regex)
	} else {
		stmt = `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata FROM node WHERE (path).path ~ $1 AND kind = $2 AND parent_id IS NOT NULL ORDER BY (path).path`
		err = r.trans.SelectContext(c, &n, stmt, regex, kind)
	}
	endQuerySpan(span, err)
	if err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) UpdateNodeFieldByID(ctx context.Context, ID int32, name string, value interface{}) error {
	stmt := fmt.Sprintf("UPDATE node set %v=$1 where ID=$2", name)
	c, span := withQuerySpan(ctx, stmt)
	if e := r.CheckFieldName(model.Node{}, name); e != nil {
		endQuerySpan(span, e)
		return e
	}
	_, err := r.trans.ExecContext(c, stmt, value, ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) MergeNodeMetadata(ctx context.Context, ID int32, metadata model.Metadata) error {
	stmt := `UPDATE node SET metadata = metadata || $1::jsonb WHERE id = $2`
	c, span := withQuerySpan(ctx, stmt)
	_, err := r.trans.ExecContext(c, stmt, metadata, ID)
	endQuerySpan(span, err)
	return err
}

// ErrKindConflict is returned when a node with the same parent and display
// name exists but with a different kind.
var ErrKindConflict = errors.New("node exists with a different kind")

func (r *SqlRepo) FindNodeByParentAndName(ctx context.Context, parentID int32, name string) (model.Node, error) {
	n := model.Node{}
	stmt := `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata
	FROM node WHERE parent_id=$1 AND display_name=$2`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &n, stmt, parentID, name)
	endQuerySpan(span, err)
	return n, err
}

// FindOrCreateLeaf finds or creates the final segment of a path. If a node
// with the same parent_id, display_name, and kind already exists it is returned
// (upsert). If a node with the same parent_id and display_name but a different
// kind exists, ErrKindConflict is returned. Otherwise a new node is created.
func (r *SqlRepo) FindOrCreateLeaf(ctx context.Context, parentID int32, name string, kind string) (model.Node, error) {
	node, err := r.FindNodeByIDAndKindAndName(ctx, parentID, kind, name)
	if err == nil {
		return node, nil
	}
	if _, conflictErr := r.FindNodeByParentAndName(ctx, parentID, name); conflictErr == nil {
		return model.Node{}, ErrKindConflict
	}
	node = model.Node{DisplayName: name, Kind: kind, ParentID: parentID}
	if err := r.CreateNode(ctx, &node); err != nil {
		return model.Node{}, err
	}
	return node, nil
}

func (r *SqlRepo) FindNodeByPath(ctx context.Context, path string) (model.Node, error) {
	n := model.Node{}
	stmt := `select id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at, coalesce(metadata, '{}') as metadata from node where (path)."path" = $1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &n, stmt, path)
	endQuerySpan(span, err)
	if err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) FindNodeByToken(ctx context.Context, token uuid.UUID) (model.Node, error) {
	n := model.Node{}
	stmt := `SELECT n.id, n.display_name, n.kind, coalesce(n.parent_id,0) as parent_id, (path).path as node_path, n.created_at, coalesce(n.metadata, '{}') as metadata FROM node n join node_token t on n.id = t.node_id WHERE t.token = $1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &n, stmt, token)
	endQuerySpan(span, err)
	if err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) FindOrCreateNodesByPath(ctx context.Context, startNodeID int32, path string, leafKind string) (model.Node, error) {
	segments := strings.Split(path, "/")
	if len(segments) == 0 {
		return model.Node{}, fmt.Errorf("empty path")
	}
	parentID := startNodeID
	for i, seg := range segments {
		if i < len(segments)-1 {
			node, err := r.FindNodeByIDAndKindAndName(ctx, parentID, "Folder", seg)
			if err != nil {
				node = model.Node{DisplayName: seg, Kind: "Folder", ParentID: parentID}
				if err := r.CreateNode(ctx, &node); err != nil {
					return model.Node{}, err
				}
			}
			parentID = node.ID
		} else {
			return r.FindOrCreateLeaf(ctx, parentID, seg, leafKind)
		}
	}
	return model.Node{}, fmt.Errorf("unreachable")
}

func (r *SqlRepo) GetNodePermission(ctx context.Context, nodeID int32, userID int32) (uint8, error) {
	var p uint8
	stmt := `SELECT get_node_permission($1,$2)`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &p, stmt, nodeID, userID)
	endQuerySpan(span, err)
	if err != nil {
		return 0, err
	}
	return p, nil
}

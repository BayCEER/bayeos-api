package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/google/uuid"
)

func (r *SqlRepo) FindNodeByID(ctx context.Context, ID int32) (model.Node, error) {
	var d model.Node
	stmt := `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id,  (path).path as node_path, created_at FROM node WHERE id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &d, stmt, ID)
	endQuerySpan(span, err)
	return d, err
}

func (r *SqlRepo) FindNodesByIDs(ctx context.Context, ids []int32) ([]model.Node, error) {
	n := []model.Node{}
	stmt := `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id,  (path).path as node_path, created_at FROM node WHERE id = ANY($1::int[])`
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
	stmt := `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id,  (path).path as node_path, created_at FROM node WHERE parent_id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &n, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) FindNodeParentsByID(ctx context.Context, ID int32) ([]model.Node, error) {
	n := []model.Node{}
	stmt := `with recursive wf as (
	SELECT id, display_name, kind, parent_id,  (path).path as node_path, created_at, 0 as level from node where id = $1
    union all
    select n.id, n.display_name, n.kind, n.parent_id, (n.path).path as node_path, n.created_at, wf.level + 1 as level FROM wf JOIN node n on n.id = wf.parent_id
) 	SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, node_path, created_at from wf order by level desc `
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
	stmt := `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at 
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
	stmt := `INSERT INTO node (display_name,kind,parent_id) VALUES ($1,$2,$3) RETURNING id,display_name,kind,coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, n, stmt,
		n.DisplayName, n.Kind, n.ParentID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) UpdateNode(ctx context.Context, n *model.Node) error {
	stmt := `UPDATE node set display_name=$1,kind=$2, parent_id=$3 where id=$4 RETURNING id,display_name,kind,coalesce(parent_id,0) as parent_id, (path).path as node_path`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, n, stmt,
		n.DisplayName, n.Kind, n.ParentID, n.ID)
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

func (r *SqlRepo) FindNodesByPath(ctx context.Context, query string, kind string) ([]model.Node, error) {
	n := []model.Node{}
	var stmt string
	c, span := withQuerySpan(ctx, "FindNodesByPath")
	var err error
	if kind == "" {
		stmt = `SELECT id, display_name, kind, parent_id, (path).path as node_path, created_at FROM node WHERE (path).path like $1 and parent_id is not null order by (path).path`
		err = r.trans.SelectContext(c, &n, stmt, fmt.Sprintf("%%%v%%", query))
	} else {
		stmt = `SELECT id, display_name, kind, parent_id, (path).path as node_path, created_at FROM node WHERE (path).path like $1 and kind = $2 and parent_id is not null order by (path).path`
		err = r.trans.SelectContext(c, &n, stmt, fmt.Sprintf("%%%v%%", query), kind)
	}
	endQuerySpan(span, err)
	if err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) FindNodesByAntPath(ctx context.Context, regex string, kind string) ([]model.Node, error) {
	n := []model.Node{}
	var stmt string
	var err error
	c, span := withQuerySpan(ctx, "FindNodesByAntPath")
	if kind == "" {
		stmt = `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at FROM node WHERE (path).path ~ $1 AND parent_id IS NOT NULL ORDER BY (path).path`
		err = r.trans.SelectContext(c, &n, stmt, regex)
	} else {
		stmt = `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at FROM node WHERE (path).path ~ $1 AND kind = $2 AND parent_id IS NOT NULL ORDER BY (path).path`
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

func (r *SqlRepo) FindNodeByPath(ctx context.Context, path string) (model.Node, error) {
	n := model.Node{}
	stmt := `select id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path, created_at from node where (path)."path" = $1`
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
	stmt := `SELECT n.id, n.display_name, n.kind, coalesce(n.parent_id,0) as parent_id, (path).path as node_path, n.created_at FROM node n join node_token t on n.id = t.node_id WHERE t.token = $1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &n, stmt, token)
	endQuerySpan(span, err)
	if err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) FindOrCreateNodesByPath(ctx context.Context, startNodeID int32, path string, leafType string) (model.Node, error) {
	kind := "Folder"
	before, after, found := strings.Cut(path, "/")
	if !found {
		kind = leafType
	}
	node, err := r.FindNodeByIDAndKindAndName(ctx, startNodeID, kind, before)
	if err != nil {
		node = model.Node{DisplayName: before, Kind: kind, ParentID: startNodeID}
		if err := r.CreateNode(ctx, &node); err != nil {
			return model.Node{}, err
		}
	}
	if found {
		return r.FindOrCreateNodesByPath(ctx, node.ID, after, leafType)
	}
	return node, nil
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

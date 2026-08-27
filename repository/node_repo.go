package repository

import (
	"fmt"
	"strings"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/google/uuid"
)

func (r *SqlRepo) FindNodeByID(ID int32) (model.Node, error) {
	var d model.Node
	err := r.trans.Get(&d, `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id,  (path).path as node_path FROM node WHERE id=$1`, ID)
	return d, err
}

func (r *SqlRepo) FindNodesByIDs(ids []int32) ([]model.Node, error) {
	n := []model.Node{}
	if err := r.trans.Select(&n, `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id,  (path).path as node_path FROM node WHERE id = ANY($1::int[])`, ids); err != nil {
		return nil, err
	}
	return n, nil
}

func (r *SqlRepo) FindNodeChildrenByID(ID int32) ([]model.Node, error) {
	n := []model.Node{}
	if err := r.trans.Select(&n, `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id,  (path).path as node_path FROM node WHERE parent_id=$1`, ID); err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) FindNodeParentsByID(ID int32) ([]model.Node, error) {
	n := []model.Node{}
	if err := r.trans.Select(&n, `with recursive wf as (
	SELECT id, display_name, kind, parent_id,  (path).path as node_path, 0 as level from node where id = $1
    union all
    select n.id, n.display_name, n.kind, n.parent_id, (n.path).path as node_path, wf.level + 1 as level FROM wf JOIN node n on n.id = wf.parent_id
) 	SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, node_path from wf order by level desc `, ID); err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) FindNodeByIDAndKindAndName(ID int32, kind string, displayName string) (model.Node, error) {
	n := model.Node{}
	if err := r.trans.Get(&n, `SELECT id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path 
	FROM node WHERE parent_id=$1 and kind=$2 and display_name=$3`, ID, kind, displayName); err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) CreateNode(n *model.Node) error {
	if err := r.trans.Get(n, `INSERT INTO node (display_name,kind,parent_id) VALUES ($1,$2,$3) RETURNING id,display_name,kind,coalesce(parent_id,0) as parent_id, (path).path as node_path`,
		n.DisplayName, n.Kind, n.ParentID); err != nil {
		return err
	}
	return nil
}

func (r *SqlRepo) UpdateNode(n *model.Node) error {
	if err := r.trans.Get(n, `UPDATE node set display_name=$1,kind=$2, parent_id=$3 where id=$4 RETURNING id,display_name,kind,coalesce(parent_id,0) as parent_id, (path).path as node_path`,
		n.DisplayName, n.Kind, n.ParentID, n.ID); err != nil {
		return err
	}
	return nil
}

func (r *SqlRepo) DeleteNodeByID(ID int32) error {
	return execDelete(r.trans, `DELETE FROM node where id = $1`, ID)
}

func (r *SqlRepo) FindNodesByPath(query string) ([]model.Node, error) {
	n := []model.Node{}
	if err := r.trans.Select(&n, `SELECT id, display_name, kind, parent_id, (path).path as node_path FROM node 	WHERE (path).path like $1`, fmt.Sprintf("%%%v%%", query)); err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) UpdateNodeFieldByID(ID int32, name string, value interface{}) error {
	if e := r.CheckFieldName(model.Node{}, name); e != nil {
		return e
	}
	_, err := r.trans.Exec(fmt.Sprintf("UPDATE node set %v=$1 where ID=$2", name), value, ID)
	return err
}

func (r *SqlRepo) FindNodeByPath(path string) (model.Node, error) {
	n := model.Node{}
	if err := r.trans.Get(&n, "select id, display_name, kind, coalesce(parent_id,0) as parent_id, (path).path as node_path from node where (path).\"path\" = $1", path); err != nil {
		return n, err
	}
	return n, nil

}

func (r *SqlRepo) FindNodeByToken(token uuid.UUID) (model.Node, error) {
	n := model.Node{}
	if err := r.trans.Get(&n, `SELECT n.id, n.display_name, n.kind, coalesce(n.parent_id,0) as parent_id, (path).path as node_path FROM node n join node_token t on n.id = t.node_id WHERE t.token = $1`, token); err != nil {
		return n, err
	}
	return n, nil
}

func (r *SqlRepo) FindOrCreateNodesByPath(startNodeID int32, path string, leafType string) (model.Node, error) {
	kind := "Folder"
	before, after, found := strings.Cut(path, "/")
	if !found {
		kind = leafType
	}
	node, err := r.FindNodeByIDAndKindAndName(startNodeID, kind, before)
	if err != nil {
		node = model.Node{DisplayName: before, Kind: kind, ParentID: startNodeID}
		if err := r.CreateNode(&node); err != nil {
			return model.Node{}, err
		}
	}
	if found {
		return r.FindOrCreateNodesByPath(node.ID, after, leafType)
	}
	return node, nil
}

func (r *SqlRepo) GetNodePermission(nodeID int32, userID int32) (uint8, error) {
	var p uint8
	if err := r.trans.Get(&p, `SELECT get_node_permission($1,$2)`, nodeID, userID); err != nil {
		return 0, err
	}
	return p, nil
}

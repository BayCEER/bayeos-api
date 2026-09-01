package repository

import (
	"context"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

func (r *SqlRepo) FindNodePrivByID(ctx context.Context, ID int32) (model.NodePrivilege, error) {
	var t model.NodePrivilege
	stmt := `SELECT * FROM node_privilege WHERE id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &t, stmt, ID)
	endQuerySpan(span, err)
	return t, err
}

func (r *SqlRepo) FindNodePrivsByGranteeID(ctx context.Context, ID int32) ([]model.NodePrivilege, error) {
	p := []model.NodePrivilege{}
	stmt := `SELECT p.* FROM node_privilege p join users u on u.id = p.grantor_id WHERE p.grantee_id=$1 and p.grantee_id != p.grantor_id order by u.display_name asc`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &p, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return p, err
	}
	return p, nil
}

func (r *SqlRepo) FindNodePrivsByGrantorID(ctx context.Context, ID int32) ([]model.NodePrivilege, error) {
	p := []model.NodePrivilege{}
	stmt := `SELECT p.* FROM node_privilege p join users u on u.id = p.grantee_id WHERE p.grantor_id=$1 and p.grantee_id != p.grantor_id order by u.display_name asc`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &p, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return p, err
	}
	return p, nil
}

func (r *SqlRepo) FindNodePrivsByNodeID(ctx context.Context, ID int32) ([]model.NodePrivilege, error) {
	p := []model.NodePrivilege{}
	stmt := `SELECT * FROM node_privilege WHERE node_id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &p, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return p, err
	}
	return p, nil
}

func (r *SqlRepo) CreateNodePriv(ctx context.Context, e *model.NodePrivilege) error {
	stmt := `INSERT INTO node_privilege (grantor_id, node_id, permission, grantee_id) VALUES ($1,$2,$3,$4) RETURNING *`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, e, stmt,
		e.GrantorID, e.NodeID, e.Permission, e.GranteeID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) UpdateNodePriv(ctx context.Context, ID int32, permission uint8) error {
	stmt := `UPDATE node_privilege set permission=$1 where id=$2`
	c, span := withQuerySpan(ctx, stmt)
	_, err := r.trans.ExecContext(c, stmt, permission, ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) DeleteNodePrivByID(ctx context.Context, ID int32) error {
	stmt := `DELETE from node_privilege where id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := execDeleteContext(c, r.trans, stmt, ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) FindNodePrivByGrantorID(ctx context.Context, ID int32) ([]model.NodePrivilege, error) {
	u := []model.NodePrivilege{}
	stmt := `SELECT id, node_id, grantee_id, permission, grantor_id from node_privilege where grantor_id = $1 order by grantee_id`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &u, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *SqlRepo) FindNodePrivByGranteeID(ctx context.Context, ID int32) ([]model.NodePrivilege, error) {
	u := []model.NodePrivilege{}
	stmt := `SELECT id, node_id, grantee_id, permission, grantor_id from node_privilege where grantee_id = $1 order by grantor_id`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &u, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return u, nil
}

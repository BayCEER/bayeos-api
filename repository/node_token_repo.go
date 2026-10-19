package repository

import (
	"context"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/google/uuid"
)

func (r *SqlRepo) FindNodeTokenByID(ctx context.Context, ID int32) (model.NodeToken, error) {
	var t model.NodeToken
	stmt := `SELECT * FROM node_token WHERE id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &t, stmt, ID)
	endQuerySpan(span, err)
	return t, err
}

// FindActiveNodeTokenByValue returns the active token with the given value.
// Inactive tokens are treated as unknown: callers must not distinguish them
// from a token that never existed.
func (r *SqlRepo) FindActiveNodeTokenByValue(ctx context.Context, token uuid.UUID) (model.NodeToken, error) {
	var t model.NodeToken
	stmt := `SELECT * FROM node_token WHERE token=$1 AND active`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &t, stmt, token)
	endQuerySpan(span, err)
	return t, err
}

func (r *SqlRepo) FindNodeTokensByNodeID(ctx context.Context, ID int32) ([]model.NodeToken, error) {
	t := []model.NodeToken{}
	stmt := `select * from node_token where node_id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &t, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return t, err
	}
	return t, nil
}

func (r *SqlRepo) CreateNodeToken(ctx context.Context, e *model.NodeToken) error {
	stmt := `INSERT INTO node_token (node_id,token,permission) VALUES ($1,$2,$3) RETURNING *`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, e, stmt,
		e.NodeID, e.Token, e.Permission)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) DeleteNodeTokenByID(ctx context.Context, ID int32) error {
	stmt := `DELETE from node_token where id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := execDeleteContext(c, r.trans, stmt, ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) UpdateNodeTokenFieldByID(ctx context.Context, ID int32, name string, value interface{}) error {
	if e := r.CheckFieldName(model.NodeToken{}, name); e != nil {
		return e
	}
	stmt := fmt.Sprintf("UPDATE node_token set %v=$1 where ID=$2", name)
	c, span := withQuerySpan(ctx, stmt)
	_, err := r.trans.ExecContext(c, stmt, value, ID)
	endQuerySpan(span, err)
	return err
}

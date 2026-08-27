package repository

import (
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

func (r *SqlRepo) FindNodeTokenByID(ID int32) (model.NodeToken, error) {
	var t model.NodeToken
	err := r.trans.Get(&t, `SELECT * FROM node_token WHERE id=$1`, ID)
	return t, err
}

func (r *SqlRepo) FindNodeTokensByNodeID(ID int32) ([]model.NodeToken, error) {
	t := []model.NodeToken{}
	if err := r.trans.Select(&t, `select * from node_token where node_id=$1`, ID); err != nil {
		return t, err
	}
	return t, nil
}

func (r *SqlRepo) CreateNodeToken(e *model.NodeToken) error {
	if err := r.trans.Get(e, `INSERT INTO node_token (node_id,token) VALUES ($1,$2) RETURNING *`,
		e.NodeID, e.Token); err != nil {
		return err
	}
	return nil
}

func (r *SqlRepo) DeleteNodeTokenByID(ID int32) error {
	return execDelete(r.trans, "DELETE from node_token where id=$1", ID)
}

func (r *SqlRepo) UpdateNodeTokenFieldByID(ID int32, name string, value interface{}) error {
	if e := r.CheckFieldName(model.NodeToken{}, name); e != nil {
		return e
	}
	_, err := r.trans.Exec(fmt.Sprintf("UPDATE node_token set %v=$1 where ID=$2", name), value, ID)
	return err
}

package repository

import (
	"github.com/BayCEER/bayeos/bayeos-api/model"
)

func (r *SqlRepo) FindNodePrivByID(ID int32) (model.NodePrivilege, error) {
	var t model.NodePrivilege
	err := r.trans.Get(&t, `SELECT * FROM node_privilege WHERE id=$1`, ID)
	return t, err
}

func (r *SqlRepo) FindNodePrivsByGranteeID(ID int32) ([]model.NodePrivilege, error) {
	p := []model.NodePrivilege{}
	if err := r.trans.Select(&p, `SELECT p.* FROM node_privilege p join users u on u.id = p.grantor_id WHERE p.grantee_id=$1 and p.grantee_id != p.grantor_id order by u.display_name asc`, ID); err != nil {
		return p, err
	}
	return p, nil
}

func (r *SqlRepo) FindNodePrivsByGrantorID(ID int32) ([]model.NodePrivilege, error) {
	p := []model.NodePrivilege{}
	if err := r.trans.Select(&p, `SELECT p.* FROM node_privilege p join users u on u.id = p.grantee_id WHERE p.grantor_id=$1 and p.grantee_id != p.grantor_id order by u.display_name asc`, ID); err != nil {
		return p, err
	}
	return p, nil
}

func (r *SqlRepo) FindNodePrivsByNodeID(ID int32) ([]model.NodePrivilege, error) {
	p := []model.NodePrivilege{}
	if err := r.trans.Select(&p, `SELECT * FROM node_privilege WHERE node_id=$1`, ID); err != nil {
		return p, err
	}
	return p, nil
}

func (r *SqlRepo) CreateNodePriv(e *model.NodePrivilege) error {
	if err := r.trans.Get(e, `INSERT INTO node_privilege (grantor_id, node_id, permission, grantee_id) VALUES ($1,$2,$3,$4) RETURNING *`,
		e.GrantorID, e.NodeID, e.Permission, e.GranteeID); err != nil {
		return err
	}
	return nil
}

func (r *SqlRepo) UpdateNodePriv(ID int32, permission uint8) error {
	_, err := r.trans.Exec(`UPDATE node_privilege set permission=$1 where id=$2`, permission, ID)
	return err
}

func (r *SqlRepo) DeleteNodePrivByID(ID int32) error {
	return execDelete(r.trans, "DELETE from node_privilege where id=$1", ID)
}

func (r *SqlRepo) FindNodePrivByGrantorID(ID int32) ([]model.NodePrivilege, error) {
	u := []model.NodePrivilege{}
	if err := r.trans.Select(&u, `SELECT id, node_id, grantee_id, permission, grantor_id from node_privileges where grantor_id = $1 order by grantee_id`, ID); err != nil {
		return nil, err
	}
	return u, nil
}

func (r *SqlRepo) FindNodePrivByGranteeID(ID int32) ([]model.NodePrivilege, error) {
	u := []model.NodePrivilege{}
	if err := r.trans.Select(&u, `SELECT id, node_id, grantee_id, permission, grantor_id from node_privileges where grantee_id = $1 order by grantor_id`, ID); err != nil {
		return nil, err
	}
	return u, nil

}

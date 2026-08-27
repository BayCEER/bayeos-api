package repository

import (
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

func (r *SqlRepo) FindUserByID(ID int32) (model.User, error) {
	var u model.User
	if err := r.trans.Get(&u, `SELECT id,display_name,sign_in_name,email_address,home_node_id 
	,password_hash,super_user,sys_user, last_sign_in_time FROM "users" WHERE id=$1`, ID); err != nil {
		return u, err
	}
	return u, nil

}

func (r *SqlRepo) FindUsersByIDs(ids []int32) ([]model.User, error) {
	u := []model.User{}
	if err := r.trans.Select(&u, `SELECT id, display_name,sign_in_name,email_address,home_node_id, 
	password_hash,super_user,sys_user, last_sign_in_time from "users" where id = ANY($1::int[]) order by display_name desc`, ids); err != nil {
		return nil, err
	}
	return u, nil
}

func (r *SqlRepo) FindUserBySignInName(signInName string) (model.User, error) {
	var u model.User
	if err := r.trans.Get(&u, `SELECT id, display_name,sign_in_name,email_address,home_node_id 
	,password_hash,super_user,sys_user,last_sign_in_time FROM "users" WHERE sign_in_name=$1`, signInName); err != nil {
		return u, err
	}
	return u, nil
}

func (r *SqlRepo) FindAllUsers() ([]model.User, error) {
	u := []model.User{}
	if err := r.trans.Select(&u, `SELECT id, display_name,sign_in_name,email_address,home_node_id, 
	password_hash,super_user,sys_user, last_sign_in_time from "users" order by display_name desc`); err != nil {
		return nil, err
	}
	return u, nil
}

func (r *SqlRepo) CreateUser(e *model.User) error {
	if err := r.trans.Get(e, `INSERT INTO "users" (display_name,sign_in_name,email_address,home_node_id 
		,password_hash,super_user,sys_user) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id,display_name,sign_in_name,email_address,home_node_id, password_hash,super_user,sys_user`,
		e.DisplayName, e.SignInName, e.EmailAddress, e.HomeNodeID, e.PasswordHash, e.SuperUser, e.SysUser); err != nil {
		return err
	}
	return nil
}

func (r *SqlRepo) UpdateUser(e *model.User) error {
	if err := r.trans.Get(e, `UPDATE "users" set display_name=$1,sign_in_name=$2,email_address=$3,home_node_id=$4, 
		,password_hash=$5,super_user=$6,sys_user=$7, last_sign_in_time=$8 WHERE id=$9 RETURNING id,display_name,sign_in_name,email_address,home_node_id, password_hash,super_user,sys_user`,
		e.DisplayName, e.SignInName, e.EmailAddress, e.HomeNodeID, e.PasswordHash, e.SuperUser, e.SysUser, e.LastSignInTime, e.ID); err != nil {
		return err
	}
	return nil
}

func (r *SqlRepo) DeleteUserByID(ID int32) error {
	return execDelete(r.db, `DELETE FROM "users" WHERE id = $1`, ID)
}

func (r *SqlRepo) UpdateUserFieldByID(ID int32, k string, v interface{}) error {
	if e := r.CheckFieldName(model.User{}, k); e != nil {
		return e
	}
	_, err := r.trans.Exec(fmt.Sprintf("UPDATE users set %v=$1 where ID=$2", k), v, ID)
	return err
}

func (r *SqlRepo) UpdatePasswordHash(signInName string, hash string) error {
	_, err := r.trans.Exec("UPDATE users set password_hash=$1 where sign_in_name=$2", hash, signInName)
	return err
}

func (r *SqlRepo) FindUserImageByID(ID int32) ([]byte, error) {
	var i []byte
	err := r.db.QueryRow("SELECT image_data FROM user_images WHERE user_id = $1", ID).Scan(&i)
	if err != nil {
		return nil, err
	}
	return i, nil
}

func (r *SqlRepo) CreateUserImage(ID int32, imageData []byte) error {
	_, err := r.trans.Exec("INSERT INTO user_images (user_id, image_data) VALUES ($1, $2)", ID, imageData)
	return err
}

func (r *SqlRepo) DeleteUserImageByID(ID int32) error {
	return execDelete(r.db, `DELETE FROM "user_images" WHERE user_id = $1`, ID)
}

package repository

import (
	"context"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

func (r *SqlRepo) FindUserByID(ctx context.Context, ID int32) (model.User, error) {
	var u model.User
	stmt := `SELECT id,display_name,sign_in_name,email_address,home_node_id 
	,password_hash,super_user,sys_user, created_at, last_sign_in_time FROM "users" WHERE id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &u, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return u, err
	}
	return u, nil
}

func (r *SqlRepo) FindUsersByIDs(ctx context.Context, ids []int32) ([]model.User, error) {
	u := []model.User{}
	stmt := `SELECT id, display_name,sign_in_name,email_address,home_node_id, 
	password_hash,super_user,sys_user, created_at, last_sign_in_time from "users" where id = ANY($1::int[]) order by display_name desc`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &u, stmt, ids)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *SqlRepo) FindUserBySignInName(ctx context.Context, signInName string) (model.User, error) {
	var u model.User
	stmt := `SELECT id, display_name,sign_in_name,email_address,home_node_id 
	,password_hash,super_user,sys_user, created_at, last_sign_in_time FROM "users" WHERE sign_in_name=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &u, stmt, signInName)
	endQuerySpan(span, err)
	if err != nil {
		return u, err
	}
	return u, nil
}

func (r *SqlRepo) FindAllUsers(ctx context.Context) ([]model.User, error) {
	u := []model.User{}
	stmt := `SELECT id, display_name,sign_in_name,email_address,home_node_id, 
	password_hash,super_user,sys_user, created_at, last_sign_in_time from "users" order by display_name desc`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &u, stmt)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *SqlRepo) CreateUser(ctx context.Context, e *model.User) error {
	stmt := `INSERT INTO "users" (display_name,sign_in_name,email_address,home_node_id 
		,password_hash,super_user,sys_user) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id,display_name,sign_in_name,email_address,home_node_id, password_hash,super_user,sys_user, created_at, last_sign_in_time`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, e, stmt,
		e.DisplayName, e.SignInName, e.EmailAddress, e.HomeNodeID, e.PasswordHash, e.SuperUser, e.SysUser)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) UpdateUser(ctx context.Context, e *model.User) error {
	stmt := `UPDATE "users" set display_name=$1,sign_in_name=$2,email_address=$3,home_node_id=$4, 
		,password_hash=$5,super_user=$6,sys_user=$7, last_sign_in_time=$8 WHERE id=$9 RETURNING id,display_name,sign_in_name,email_address,home_node_id, password_hash,super_user,sys_user, created_at, last_sign_in_time`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, e, stmt,
		e.DisplayName, e.SignInName, e.EmailAddress, e.HomeNodeID, e.PasswordHash, e.SuperUser, e.SysUser, e.LastSignInTime, e.ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) DeleteUserByID(ctx context.Context, ID int32) error {
	stmt := `DELETE FROM "users" WHERE id = $1`
	c, span := withQuerySpan(ctx, stmt)
	err := execDeleteContext(c, r.db, stmt, ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) UpdateUserFieldByID(ctx context.Context, ID int32, k string, v interface{}) error {

	stmt := fmt.Sprintf("UPDATE users set %v=$1 where ID=$2", k)
	c, span := withQuerySpan(ctx, stmt)
	if e := r.CheckFieldName(model.User{}, k); e != nil {
		endQuerySpan(span, e)
		return e
	}
	_, err := r.trans.ExecContext(c, stmt, v, ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) UpdatePasswordHash(ctx context.Context, signInName string, hash string) error {
	stmt := "UPDATE users set password_hash=$1 where sign_in_name=$2"
	c, span := withQuerySpan(ctx, stmt)
	_, err := r.trans.ExecContext(c, stmt, hash, signInName)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) FindUserImageByID(ctx context.Context, ID int32) ([]byte, string, error) {
	var i []byte
	var mimeType string
	stmt := `SELECT image_data, mime_type FROM user_images WHERE user_id = $1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.db.QueryRowContext(c, stmt, ID).Scan(&i, &mimeType)
	endQuerySpan(span, err)
	if err != nil {
		return nil, "", err
	}
	return i, mimeType, nil
}

func (r *SqlRepo) CreateUserImage(ctx context.Context, ID int32, imageData []byte, mimeType string) error {
	stmt := `INSERT INTO user_images (user_id, image_data, mime_type) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET image_data = EXCLUDED.image_data, mime_type = EXCLUDED.mime_type`
	c, span := withQuerySpan(ctx, stmt)
	_, err := r.trans.ExecContext(c, stmt, ID, imageData, mimeType)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) DeleteUserImageByID(ctx context.Context, ID int32) error {
	stmt := `DELETE FROM "user_images" WHERE user_id = $1`
	c, span := withQuerySpan(ctx, stmt)
	err := execDeleteContext(c, r.db, stmt, ID)
	endQuerySpan(span, err)
	return err
}

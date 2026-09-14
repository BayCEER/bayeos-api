package repository

import (
	"context"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

func (r *SqlRepo) FindUserGroupByID(ctx context.Context, ID int32) (model.UserGroup, error) {
	var g model.UserGroup
	stmt := `SELECT id, display_name, sys_group, created_at FROM user_group WHERE id=$1`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, &g, stmt, ID)
	endQuerySpan(span, err)
	if err != nil {
		return g, err
	}
	return g, nil
}

func (r *SqlRepo) FindUserGroupsByIDs(ctx context.Context, ids []int32) ([]model.UserGroup, error) {
	g := []model.UserGroup{}
	stmt := `SELECT id, display_name, sys_group, created_at FROM user_group WHERE id = ANY($1::int[]) ORDER BY display_name`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &g, stmt, ids)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *SqlRepo) FindAllUserGroups(ctx context.Context) ([]model.UserGroup, error) {
	g := []model.UserGroup{}
	stmt := `SELECT id, display_name, sys_group, created_at FROM user_group ORDER BY display_name`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &g, stmt)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *SqlRepo) CreateUserGroup(ctx context.Context, g *model.UserGroup) error {
	stmt := `INSERT INTO user_group (display_name, sys_group) VALUES ($1,$2) RETURNING id, display_name, sys_group, created_at`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(c, g, stmt, g.DisplayName, g.SysGroup)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) UpdateUserGroupFieldByID(ctx context.Context, ID int32, k string, v interface{}) error {
	stmt := fmt.Sprintf("UPDATE user_group set %v=$1 where ID=$2", k)
	c, span := withQuerySpan(ctx, stmt)
	if e := r.CheckFieldName(model.UserGroup{}, k); e != nil {
		endQuerySpan(span, e)
		return e
	}
	_, err := r.trans.ExecContext(c, stmt, v, ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) DeleteUserGroupByID(ctx context.Context, ID int32) error {
	stmt := `DELETE FROM user_group where id = $1`
	c, span := withQuerySpan(ctx, stmt)
	err := execDeleteContext(c, r.trans, stmt, ID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) FindUserGroupMemberIDs(ctx context.Context, groupID int32) ([]int32, error) {
	ids := []int32{}
	stmt := `SELECT user_id FROM user_group_member WHERE group_id=$1 ORDER BY user_id`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &ids, stmt, groupID)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *SqlRepo) FindMemberUsersByGroupID(ctx context.Context, groupID int32) ([]model.User, error) {
	u := []model.User{}
	stmt := `SELECT u.id, u.display_name, u.sign_in_name, u.email_address, u.home_node_id
	, u.password_hash, u.super_user, u.sys_user, u.created_at, u.last_sign_in_time
	FROM user_group_member m JOIN "users" u ON m.user_id = u.id WHERE m.group_id=$1 ORDER BY u.display_name`
	c, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(c, &u, stmt, groupID)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *SqlRepo) CreateUserGroupMembers(ctx context.Context, groupID int32, userIDs []int32) error {
	stmt := `INSERT INTO user_group_member (group_id, user_id) SELECT $1, u FROM unnest($2::int[]) AS u
	ON CONFLICT (group_id, user_id) DO NOTHING`
	c, span := withQuerySpan(ctx, stmt)
	_, err := r.trans.ExecContext(c, stmt, groupID, userIDs)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) DeleteUserGroupMember(ctx context.Context, groupID int32, userID int32) error {
	stmt := `DELETE FROM user_group_member WHERE group_id=$1 AND user_id=$2`
	c, span := withQuerySpan(ctx, stmt)
	err := execDeleteContext(c, r.trans, stmt, groupID, userID)
	endQuerySpan(span, err)
	return err
}

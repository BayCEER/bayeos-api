package service

import (
	"context"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
)

type UserGroupService interface {
	GetUserGroup(ctx context.Context, id int32) (model.UserGroup, error)
	GetUserGroups(ctx context.Context, ids []int32) ([]model.UserGroup, error)
	CreateUserGroup(ctx context.Context, u *model.UserGroup) error
	UpdateUserGroup(ctx context.Context, id int32, fields *map[string]interface{}) error
	DeleteUserGroup(ctx context.Context, id int32) error

	GetUserGroupMembers(ctx context.Context, groupID int32) ([]model.User, error)
	AddUserGroupMembers(ctx context.Context, groupID int32, userIDs []int32) error
	RemoveUserGroupMember(ctx context.Context, groupID int32, userID int32) error

	GetUserGroupPrivileges(ctx context.Context, groupID int32) ([]model.NodePrivilege, error)
}

type userGroupService struct {
	sqlRepo repository.SqlRepo
}

func NewUserGroupService(sqlRepo repository.SqlRepo) UserGroupService {
	return &userGroupService{
		sqlRepo: sqlRepo,
	}
}

func (s *userGroupService) GetUserGroup(ctx context.Context, id int32) (g model.UserGroup, err error) {
	ctx, span := withServiceSpan(ctx, "user_groups.get")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)

	g, err = s.sqlRepo.FindUserGroupByID(ctx, id)
	if err != nil {
		return g, fmt.Errorf("user group with id %d not found: %w", id, err)
	}

	err = s.fillMemberIDs(ctx, &g)
	if err != nil {
		return g, fmt.Errorf("failed to load members of user group %d: %w", id, err)
	}
	return g, nil
}

func (s *userGroupService) GetUserGroups(ctx context.Context, ids []int32) (groups []model.UserGroup, err error) {
	ctx, span := withServiceSpan(ctx, "user_groups.list")
	defer func() { endServiceSpan(span, err) }()

	if len(ids) != 0 {
		groups, err = s.sqlRepo.FindUserGroupsByIDs(ctx, ids)
	} else {
		groups, err = s.sqlRepo.FindAllUserGroups(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user groups: %w", err)
	}
	for i := range groups {
		if err := s.fillMemberIDs(ctx, &groups[i]); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

func (s *userGroupService) CreateUserGroup(ctx context.Context, ug *model.UserGroup) (err error) {
	ctx, span := withServiceSpan(ctx, "user_groups.create")
	defer func() { endServiceSpan(span, err) }()

	if err := s.sqlRepo.CreateUserGroup(ctx, ug); err != nil {
		return fmt.Errorf("failed to create user group: %w", err)
	}
	return nil
}

func (s *userGroupService) UpdateUserGroup(ctx context.Context, id int32, fields *map[string]interface{}) (err error) {
	ctx, span := withServiceSpan(ctx, "user_groups.update")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)

	if err := s.sqlRepo.Do(ctx, func(repo repository.SqlRepo) error {
		for k, v := range *fields {
			if err := s.sqlRepo.UpdateUserGroupFieldByID(ctx, id, k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update user group: %w", err)
	}
	return nil
}

func (s *userGroupService) DeleteUserGroup(ctx context.Context, id int32) (err error) {
	ctx, span := withServiceSpan(ctx, "user_groups.delete")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, id)

	if err := s.sqlRepo.DeleteUserGroupByID(ctx, id); err != nil {
		return fmt.Errorf("failed to delete user group: %w", err)
	}
	return nil
}

func (s *userGroupService) GetUserGroupMembers(ctx context.Context, groupID int32) (users []model.User, err error) {
	ctx, span := withServiceSpan(ctx, "user_groups.members")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, groupID)

	users, err = s.sqlRepo.FindMemberUsersByGroupID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("members of user group %d not found: %w", groupID, err)
	}
	return users, nil
}

func (s *userGroupService) AddUserGroupMembers(ctx context.Context, groupID int32, userIDs []int32) (err error) {
	ctx, span := withServiceSpan(ctx, "user_groups.add_members")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, groupID)

	if len(userIDs) == 0 {
		return nil
	}
	if err := s.sqlRepo.Do(ctx, func(repo repository.SqlRepo) error {
		return repo.CreateUserGroupMembers(ctx, groupID, userIDs)
	}); err != nil {
		return fmt.Errorf("failed to add members to user group %d: %w", groupID, err)
	}
	return nil
}

func (s *userGroupService) RemoveUserGroupMember(ctx context.Context, groupID int32, userID int32) (err error) {
	ctx, span := withServiceSpan(ctx, "user_groups.remove_member")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, groupID)

	if err := s.sqlRepo.DeleteUserGroupMember(ctx, groupID, userID); err != nil {
		return fmt.Errorf("failed to remove member %d from user group %d: %w", userID, groupID, err)
	}
	return nil
}

func (s *userGroupService) GetUserGroupPrivileges(ctx context.Context, groupID int32) (privs []model.NodePrivilege, err error) {
	ctx, span := withServiceSpan(ctx, "user_groups.privileges")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, groupID)

	privs, err = s.sqlRepo.FindNodePrivsByGroupID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("privileges of user group %d not found: %w", groupID, err)
	}
	return privs, nil
}

func (s *userGroupService) fillMemberIDs(ctx context.Context, g *model.UserGroup) error {
	ids, err := s.sqlRepo.FindUserGroupMemberIDs(ctx, g.ID)
	if err != nil {
		return err
	}
	g.MemberIDs = ids
	return nil
}

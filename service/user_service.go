package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
)

type UserService interface {
	CreateUser(ctx context.Context, cre model.SignUpCredentials) (model.User, error)
	GetUser(ctx context.Context, id int32) (model.User, error)
	GetUsers(ctx context.Context, ids []int32) ([]model.User, error)
	GetAllUsers(ctx context.Context) ([]model.User, error)

	UpdateUser(ctx context.Context, id int32, fields *map[string]interface{}) error
	DeleteUser(ctx context.Context, id int32) error

	SetPassword(ctx context.Context, sc model.SignInCredentials) error
	SignIn(ctx context.Context, sc model.SignInCredentials) (model.User, error)
	SignOut(ctx context.Context, sc model.SignInCredentials) error

	CreateUserImage(ctx context.Context, id int32, imageData io.Reader) error
	GetUserImage(ctx context.Context, id int32) ([]byte, error)
	DeleteUserImage(ctx context.Context, id int32) error
}

type userService struct {
	sqlRepo  repository.SqlRepo
	ldapRepo repository.LdapRepo
}

const MIN_PASSWORD_LENGTH = 5

func NewUserService(sqlRepo repository.SqlRepo, ldapRepo repository.LdapRepo) UserService {
	return &userService{
		sqlRepo:  sqlRepo,
		ldapRepo: ldapRepo,
	}
}

func (s *userService) CreateUser(tx context.Context, cre model.SignUpCredentials) (model.User, error) {

	if len(cre.Password) < MIN_PASSWORD_LENGTH {
		return model.User{}, errors.New("password length too short")
	}

	var u model.User

	if err := s.sqlRepo.Do(tx, func(repo repository.SqlRepo) error {

		// Get users home folder
		homeNode, err := repo.FindNodeByID(model.HOME_ID)
		if err != nil {
			return err
		}

		// Create new folder, raises an error if a node with the same name already exists !
		node := model.Node{DisplayName: cre.DisplayName, ParentID: homeNode.ID, Kind: "Folder"}
		if err := repo.CreateNode(&node); err != nil {
			return err
		}

		// Create user
		u = model.User{DisplayName: cre.DisplayName, EmailAddress: cre.EmailAddress, SignInName: cre.SignInName,
			PasswordHash: s.encodePassword(cre.Password), HomeNodeID: node.ID}
		if err := repo.CreateUser(&u); err != nil {
			return err
		}

		// Grant privilege to user
		p := model.NewNodePrivilege(node.ID, u.ID, u.ID)
		if err := repo.CreateNodePriv(&p); err != nil {
			return err
		}

		return nil

	}); err != nil {
		return u, fmt.Errorf("failed to save user:%s", cre.DisplayName)
	}
	return u, nil
}

func (s *userService) DeleteUser(ctx context.Context, id int32) error {
	return s.sqlRepo.DeleteUserByID(id)
}

func (s *userService) SetPassword(ctx context.Context, sc model.SignInCredentials) error {
	return s.sqlRepo.UpdatePasswordHash(sc.Name, s.encodePassword(sc.Password))
}

func (s *userService) GetUsers(ctx context.Context, ids []int32) ([]model.User, error) {
	if len(ids) != 0 {
		return s.sqlRepo.FindUsersByIDs(ids)
	}
	return s.sqlRepo.FindAllUsers()
}

func (s *userService) GetAllUsers(ctx context.Context) ([]model.User, error) {
	return s.sqlRepo.FindAllUsers()
}

func (s *userService) UpdateUser(ctx context.Context, id int32, fields *map[string]interface{}) error {

	if err := s.sqlRepo.Do(context.Background(), func(repo repository.SqlRepo) error {
		for k, v := range *fields {
			if err := s.sqlRepo.UpdateUserFieldByID(id, k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil

}

func (s *userService) GetUser(ctx context.Context, id int32) (model.User, error) {
	return s.sqlRepo.FindUserByID(id)
}

func (s *userService) SignIn(ctx context.Context, cre model.SignInCredentials) (model.User, error) {
	u, err := s.sqlRepo.FindUserBySignInName(cre.Name)
	if err == nil {
		// User exists
		// Try local auth
		localAuth := u.PasswordHash == s.encodePassword(cre.Password)
		// Try ldap auth
		ldapAuth := false
		if s.ldapRepo.Enabled() {
			if b, err := s.ldapRepo.Authenticate(cre.Name, cre.Password); err == nil && b {
				// LDAP auth success
				ldapAuth = true
			}
		}
		if localAuth || ldapAuth {
			s.sqlRepo.UpdateUserFieldByID(u.ID, "last_sign_in_time", time.Now())
			return u, nil
		}
	}
	// Default
	return model.User{}, errors.New("user authentication failed")
}

func (s *userService) SignOut(ctx context.Context, cre model.SignInCredentials) error {

	u, err := s.SignIn(ctx, cre)
	if err != nil {
		return err
	}

	// Not allowed for users with system role
	if u.SysUser {
		return errors.New("not allowed for system users")
	}

	// None system users
	if err := s.sqlRepo.Do(ctx, func(repo repository.SqlRepo) error {

		if err = s.sqlRepo.DeleteUserByID(u.ID); err != nil {
			return err
		}
		return nil

	}); err != nil {
		return fmt.Errorf("failed to sign out user:%s", cre.Name)
	}

	return nil
}

func (s *userService) CreateUserImage(ctx context.Context, id int32, imageData io.Reader) error {

	data, err := io.ReadAll(imageData)
	if err != nil {
		return err
	}

	err = s.sqlRepo.CreateUserImage(id, data)
	if err != nil {
		return err
	}

	return nil
}

func (s *userService) GetUserImage(ctx context.Context, id int32) ([]byte, error) {
	return s.sqlRepo.FindUserImageByID(id)
}

func (s *userService) DeleteUserImage(ctx context.Context, id int32) error {
	return s.sqlRepo.DeleteUserImageByID(id)
}

func (s *userService) encodePassword(password string) string {
	h := md5.New()
	io.WriteString(h, password)
	return hex.EncodeToString(h.Sum(nil))
}

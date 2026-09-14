package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/ldap"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/gabriel-vasile/mimetype"
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
	GetUserImage(ctx context.Context, id int32) ([]byte, string, error)
	DeleteUserImage(ctx context.Context, id int32) error
}

type userService struct {
	sqlRepo  repository.SqlRepo
	ldapRepo ldap.LdapRepo
}

const MIN_PASSWORD_LENGTH = 5

const MaxUserImageBytes = 5 << 20

var ErrImageTooLarge = errors.New("image exceeds maximum allowed size")
var ErrUnsupportedImageType = errors.New("unsupported image type, only PNG and JPEG are allowed")

func NewUserService(sqlRepo repository.SqlRepo, ldapRepo ldap.LdapRepo) UserService {
	return &userService{
		sqlRepo:  sqlRepo,
		ldapRepo: ldapRepo,
	}
}

func (s *userService) CreateUser(tx context.Context, cre model.SignUpCredentials) (u model.User, err error) {
	ctx, span := withServiceSpan(tx, "users.create")
	defer func() { endServiceSpan(span, err) }()

	if len(cre.Password) < MIN_PASSWORD_LENGTH {
		return model.User{}, errors.New("password length too short")
	}

	if err := s.sqlRepo.Do(ctx, func(repo repository.SqlRepo) error {

		// Get users home folder
		homeNode, err := repo.FindNodeByID(ctx, model.HOME_ID)
		if err != nil {
			return err
		}

		// Create new folder, raises an error if a node with the same name already exists !
		node := model.Node{DisplayName: cre.DisplayName, ParentID: homeNode.ID, Kind: "Folder"}
		if err := repo.CreateNode(ctx, &node); err != nil {
			return err
		}

		hash, err := s.encodeMD5(cre.Password)
		if err != nil {
			return fmt.Errorf("failed to encode MD5 password for user:%s", cre.DisplayName)
		}

		// Create user
		u = model.User{DisplayName: cre.DisplayName, EmailAddress: cre.EmailAddress, SignInName: cre.SignInName,
			PasswordHash: hash, HomeNodeID: node.ID}
		if err := repo.CreateUser(ctx, &u); err != nil {
			return err
		}

		// Grant privilege to user
		p := model.NewNodePrivilege(node.ID, u.ID, u.ID)
		if err := repo.CreateNodePriv(ctx, &p); err != nil {
			return err
		}

		return nil

	}); err != nil {
		return u, fmt.Errorf("failed to save user:%s", cre.DisplayName)
	}
	return u, nil
}

func (s *userService) DeleteUser(ctx context.Context, id int32) (err error) {
	ctx, span := withServiceSpan(ctx, "users.delete")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, id)

	return s.sqlRepo.DeleteUserByID(ctx, id)
}

func (s *userService) SetPassword(ctx context.Context, sc model.SignInCredentials) (err error) {
	ctx, span := withServiceSpan(ctx, "users.set_password")
	defer func() { endServiceSpan(span, err) }()

	if hash, err := s.encodeMD5(sc.Password); err != nil {
		return fmt.Errorf("failed to encode MD5 password for user:%s", sc.Name)
	} else {
		return s.sqlRepo.UpdatePasswordHash(ctx, sc.Name, hash)
	}

}

func (s *userService) GetUsers(ctx context.Context, ids []int32) (users []model.User, err error) {
	ctx, span := withServiceSpan(ctx, "users.list")
	defer func() { endServiceSpan(span, err) }()

	if len(ids) != 0 {
		return s.sqlRepo.FindUsersByIDs(ctx, ids)
	}
	return s.sqlRepo.FindAllUsers(ctx)
}

func (s *userService) GetAllUsers(ctx context.Context) (users []model.User, err error) {
	ctx, span := withServiceSpan(ctx, "users.list_all")
	defer func() { endServiceSpan(span, err) }()

	return s.sqlRepo.FindAllUsers(ctx)
}

func (s *userService) UpdateUser(ctx context.Context, id int32, fields *map[string]interface{}) (err error) {
	ctx, span := withServiceSpan(ctx, "users.update")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, id)

	if err := s.sqlRepo.Do(ctx, func(repo repository.SqlRepo) error {
		for k, v := range *fields {
			if err := s.sqlRepo.UpdateUserFieldByID(ctx, id, k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil

}

func (s *userService) GetUser(ctx context.Context, id int32) (u model.User, err error) {
	ctx, span := withServiceSpan(ctx, "users.get")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, id)

	return s.sqlRepo.FindUserByID(ctx, id)
}

func (s *userService) SignIn(ctx context.Context, cre model.SignInCredentials) (u model.User, err error) {
	ctx, span := withServiceSpan(ctx, "users.sign_in")
	defer func() { endServiceSpan(span, err) }()

	u, err = s.sqlRepo.FindUserBySignInName(ctx, cre.Name)
	if err == nil {
		// User exists

		// Try local auth
		localAuth := false
		if len(u.PasswordHash) == 32 {
			if hash, err := s.encodeMD5(cre.Password); err == nil {
				localAuth = u.PasswordHash == hash
			} else {
				return model.User{}, fmt.Errorf("failed to encode MD5 password for user:%s", u.SignInName)
			}

		} else if len(u.PasswordHash) == 13 {
			// Upgrade from DES hash to MD5 hash on first successful login
			if hash, err := s.encodeDES(cre.Password, u.PasswordHash); err != nil {
				return model.User{}, fmt.Errorf("failed to encode DES hash for user:%s", u.SignInName)
			} else {
				localAuth = u.PasswordHash == hash
				// DES hash -- Update to MD5 hash when user signs in successfully
				if localAuth {
					// Update to MD5 hash
					if hash, err = s.encodeMD5(cre.Password); err != nil {
						return model.User{}, fmt.Errorf("failed to md5 encode password for user:%s", u.SignInName)
					}
					if err := s.sqlRepo.UpdatePasswordHash(ctx, u.SignInName, hash); err != nil {
						return model.User{}, fmt.Errorf("failed to update password hash for user:%s", u.SignInName)
					}
				}
			}

		}

		// Try ldap auth
		ldapAuth := false
		if s.ldapRepo.Enabled() && !localAuth {
			if b, err := s.ldapRepo.Authenticate(ctx, cre.Name, cre.Password); err == nil && b {
				// LDAP auth success
				ldapAuth = true
			} else {
				return model.User{}, fmt.Errorf("failed to authenticate user:%s via LDAP: %v", u.SignInName, err)
			}
		}

		if localAuth || ldapAuth {
			if err = s.sqlRepo.UpdateUserFieldByID(ctx, u.ID, "last_sign_in_time", time.Now()); err != nil {
				return model.User{}, err
			}
			return u, nil
		}
	}
	// Default
	return model.User{}, errors.New("user authentication failed")
}

func (s *userService) SignOut(ctx context.Context, cre model.SignInCredentials) (err error) {
	ctx, span := withServiceSpan(ctx, "users.sign_out")
	defer func() { endServiceSpan(span, err) }()

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

		if err = s.sqlRepo.DeleteUserByID(ctx, u.ID); err != nil {
			return err
		}
		return nil

	}); err != nil {
		return fmt.Errorf("failed to sign out user:%s", cre.Name)
	}

	return nil
}

func (s *userService) CreateUserImage(ctx context.Context, id int32, imageData io.Reader) (err error) {
	ctx, span := withServiceSpan(ctx, "users.create_image")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, id)

	data, err := io.ReadAll(io.LimitReader(imageData, MaxUserImageBytes+1))
	if err != nil {
		return err
	}

	if len(data) > MaxUserImageBytes {
		return ErrImageTooLarge
	}

	mimeType := mimetype.Detect(data).String()
	if mimeType != "image/jpeg" && mimeType != "image/png" {
		return ErrUnsupportedImageType
	}

	err = s.sqlRepo.CreateUserImage(ctx, id, data, mimeType)
	if err != nil {
		return err
	}

	return nil
}

func (s *userService) GetUserImage(ctx context.Context, id int32) (data []byte, mimeType string, err error) {
	ctx, span := withServiceSpan(ctx, "users.get_image")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, id)

	return s.sqlRepo.FindUserImageByID(ctx, id)
}

func (s *userService) DeleteUserImage(ctx context.Context, id int32) (err error) {
	ctx, span := withServiceSpan(ctx, "users.delete_image")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, id)

	return s.sqlRepo.DeleteUserImageByID(ctx, id)
}

func (s *userService) encodeMD5(str string) (string, error) {
	h := md5.New()
	n, err := io.WriteString(h, str)
	if err != nil {
		return "", fmt.Errorf("failed to write hash: %v", err)
	}
	if n != len(str) {
		return "", fmt.Errorf("incomplete write to hash: wrote %d bytes, expected %d bytes", n, len(str))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *userService) encodeDES(str string, salt string) (string, error) {
	if len(str) < 2 {
		return "", errors.New("string length too short")
	}
	hash, err := CryptDES(str, salt)
	if err != nil {
		return "", errors.New("failed to encode string with DES")
	}
	return hash, nil
}

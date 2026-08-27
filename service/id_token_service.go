package service

import (
	"fmt"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/dgrijalva/jwt-go"
)

type IDTokenService interface {
	CreateToken(u model.User) (string, error)
	ValidateToken(token string) (model.User, error)
}

type idTokenService struct {
	APIKey         string
	ExpirationSecs uint
}

type idTokenCustomClaims struct {
	User *model.User `json:"user"`
	jwt.StandardClaims
}

type TSConfig struct {
	APIKey            string
	ExpirationMinutes uint
}

func NewTokenService(c *TSConfig) IDTokenService {
	return &idTokenService{
		APIKey:         c.APIKey,
		ExpirationSecs: c.ExpirationMinutes * 60,
	}
}

func (s *idTokenService) CreateToken(u model.User) (string, error) {
	unixTime := time.Now().Unix()
	tokenExp := unixTime + int64(s.ExpirationSecs)
	claims := idTokenCustomClaims{
		User: &u,
		StandardClaims: jwt.StandardClaims{
			IssuedAt:  unixTime,
			ExpiresAt: tokenExp,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(s.APIKey))
	return ss, err
}

func (s *idTokenService) ValidateToken(tokenString string) (model.User, error) {
	claims := &idTokenCustomClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.APIKey), nil
	})

	user := model.User{}
	if err != nil {
		return user, err
	}

	if !token.Valid {
		return user, fmt.Errorf("ID token is invalid")
	}

	claims, ok := token.Claims.(*idTokenCustomClaims)
	if !ok {
		return user, fmt.Errorf("ID token valid but couldn't parse claims")
	}
	return *claims.User, nil
}

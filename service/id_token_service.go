package service

import (
	"context"
	"fmt"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/dgrijalva/jwt-go"
)

type IDTokenService interface {
	CreateToken(ctx context.Context, u model.User) (string, error)
	ValidateToken(ctx context.Context, token string) (model.User, error)
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

func (s *idTokenService) CreateToken(ctx context.Context, u model.User) (ss string, err error) {
	ctx, span := withServiceSpan(ctx, "id_token.create")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, u.ID)

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
	ss, err = token.SignedString([]byte(s.APIKey))
	return ss, err
}

func (s *idTokenService) ValidateToken(ctx context.Context, tokenString string) (user model.User, err error) {
	ctx, span := withServiceSpan(ctx, "id_token.validate")
	defer func() { endServiceSpan(span, err) }()

	claims := &idTokenCustomClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.APIKey), nil
	})

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

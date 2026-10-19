package service

import (
	"context"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/google/uuid"
)

type NodeTokenService interface {
	GetNodeToken(ctx context.Context, id int32) (model.NodeToken, error)
	// FindNodeTokenByValue returns the active token with the given value. It is
	// the authentication lookup: inactive tokens are not returned.
	FindNodeTokenByValue(ctx context.Context, token uuid.UUID) (model.NodeToken, error)
	CreateNodeToken(ctx context.Context, e *model.NodeToken) error
	DeleteNodeToken(ctx context.Context, id int32) error
	UpdateNodeToken(ctx context.Context, id int32, fields *map[string]interface{}) error
}

type nodeTokenService struct {
	sqlRepo repository.SqlRepo
}

func NewNodeTokenService(sqlRepo repository.SqlRepo) NodeTokenService {
	return &nodeTokenService{
		sqlRepo: sqlRepo,
	}
}

func (s *nodeTokenService) GetNodeToken(ctx context.Context, id int32) (t model.NodeToken, err error) {
	ctx, span := withServiceSpan(ctx, "node_token.get")
	defer func() { endServiceSpan(span, err) }()

	return s.sqlRepo.FindNodeTokenByID(ctx, id)
}

func (s *nodeTokenService) FindNodeTokenByValue(ctx context.Context, token uuid.UUID) (t model.NodeToken, err error) {
	ctx, span := withServiceSpan(ctx, "node_token.find_by_value")
	defer func() { endServiceSpan(span, err) }()

	return s.sqlRepo.FindActiveNodeTokenByValue(ctx, token)
}

func (s *nodeTokenService) CreateNodeToken(ctx context.Context, e *model.NodeToken) (err error) {
	ctx, span := withServiceSpan(ctx, "node_token.create")
	defer func() { endServiceSpan(span, err) }()
	setNodeID(span, e.NodeID)

	return s.sqlRepo.CreateNodeToken(ctx, e)
}

func (s *nodeTokenService) DeleteNodeToken(ctx context.Context, id int32) (err error) {
	ctx, span := withServiceSpan(ctx, "node_token.delete")
	defer func() { endServiceSpan(span, err) }()

	return s.sqlRepo.DeleteNodeTokenByID(ctx, id)
}

func (s *nodeTokenService) UpdateNodeToken(ctx context.Context, id int32, fields *map[string]interface{}) (err error) {
	ctx, span := withServiceSpan(ctx, "node_token.update")
	defer func() { endServiceSpan(span, err) }()

	if err := s.sqlRepo.Do(ctx, func(repo repository.SqlRepo) error {
		for k, v := range *fields {
			if err := s.sqlRepo.UpdateNodeTokenFieldByID(ctx, id, k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil

}

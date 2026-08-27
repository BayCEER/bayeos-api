package service

import (
	"context"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
)

type NodeTokenService interface {
	GetNodeToken(ctx context.Context, id int32) (model.NodeToken, error)
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

func (s *nodeTokenService) GetNodeToken(ctx context.Context, id int32) (model.NodeToken, error) {
	return s.sqlRepo.FindNodeTokenByID(id)
}

func (s *nodeTokenService) CreateNodeToken(ctx context.Context, e *model.NodeToken) error {
	return s.sqlRepo.CreateNodeToken(e)
}

func (s *nodeTokenService) DeleteNodeToken(ctx context.Context, id int32) error {
	return s.sqlRepo.DeleteNodeTokenByID(id)
}

func (s *nodeTokenService) UpdateNodeToken(ctx context.Context, id int32, fields *map[string]interface{}) error {

	if err := s.sqlRepo.Do(context.Background(), func(repo repository.SqlRepo) error {
		for k, v := range *fields {
			if err := s.sqlRepo.UpdateNodeTokenFieldByID(id, k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil

}

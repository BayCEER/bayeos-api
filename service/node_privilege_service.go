package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
)

type NodePrivService interface {
	GetNodePriv(ctx context.Context, id int32) (model.NodePrivilege, error)
	CreateNodePriv(ctx context.Context, nodePriv *model.NodePrivilege) error
	DeleteNodePriv(ctx context.Context, id int32) error
	UpdateNodePriv(ctx context.Context, id int32, permission uint8) error
	GetNodePrivsByUserID(ctx context.Context, id int32) ([]model.NodePrivilege, error)
}

type nodePrivService struct {
	sqlRepo repository.SqlRepo
}

func NewNodePrivService(sqlRepo repository.SqlRepo) NodePrivService {
	return &nodePrivService{
		sqlRepo: sqlRepo,
	}
}

func (s *nodePrivService) GetNodePriv(ctx context.Context, id int32) (model.NodePrivilege, error) {
	n, err := s.sqlRepo.FindNodePrivByID(ctx, id)
	if err != nil {
		return n, fmt.Errorf("node_privilege with id %d not found: %w", id, err)
	}
	return n, nil
}

func (s *nodePrivService) CreateNodePriv(ctx context.Context, nodePriv *model.NodePrivilege) error {
	if err := s.sqlRepo.CreateNodePriv(ctx, nodePriv); err != nil {
		return errors.New("failed to create node privilege")
	}
	return nil
}

func (s *nodePrivService) DeleteNodePriv(ctx context.Context, id int32) error {
	if err := s.sqlRepo.DeleteNodePrivByID(ctx, id); err != nil {
		return fmt.Errorf("failed to delete node privilege: %w", err)
	}
	return nil
}

func (s *nodePrivService) UpdateNodePriv(ctx context.Context, id int32, permission uint8) error {
	if err := s.sqlRepo.UpdateNodePriv(ctx, id, permission); err != nil {
		return errors.New("failed to update node privilege")
	}
	return nil
}

func (s *nodePrivService) GetNodePrivsByUserID(ctx context.Context, id int32) ([]model.NodePrivilege, error) {
	n, err := s.sqlRepo.FindNodePrivsByUserID(ctx, id)
	if err != nil {
		return n, fmt.Errorf("node privileges for user %d not found: %w", id, err)
	}
	return n, nil
}

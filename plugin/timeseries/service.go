package timeseries

import (
	"context"
	"errors"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
)

// ErrNotTimeseries is returned when a node exists but is not a Timeseries.
var ErrNotTimeseries = errors.New("node is not a timeseries")

// TimeseriesService exposes the plugin's operations on top of the shared node
// tree and the QuestDB data repo.
type TimeseriesService interface {
	Create(ctx context.Context, cre TimeseriesCreate, userID int32) (model.Node, error)
	Get(ctx context.Context, id int32, userID int32) (model.Node, error)
	Update(ctx context.Context, id int32, fields map[string]interface{}, userID int32) error
	Delete(ctx context.Context, id int32, userID int32) error

	AddData(ctx context.Context, id int32, pts []DataPoint, userID int32) error
	QueryData(ctx context.Context, id int32, q TimeseriesQuery, userID int32) ([]DataPoint, error)
}

type timeseriesService struct {
	nodes  service.NodeService
	privs  service.NodePrivService
	queries QuestRepo
}

// NewTimeseriesService returns a TimeseriesService backed by node/priv services
// and a QuestRepo for data.
func NewTimeseriesService(nodes service.NodeService, privs service.NodePrivService, queries QuestRepo) TimeseriesService {
	return &timeseriesService{nodes: nodes, privs: privs, queries: queries}
}

func (s *timeseriesService) Create(ctx context.Context, cre TimeseriesCreate, userID int32) (model.Node, error) {
	// Verify the parent exists and is readable by the caller.
	if _, err := s.nodes.GetNode(ctx, cre.ParentID, userID); err != nil {
		return model.Node{}, fmt.Errorf("parent node %d not found or not readable: %w", cre.ParentID, err)
	}
	node := model.Node{DisplayName: cre.DisplayName, Kind: Kind, ParentID: cre.ParentID}
	if err := s.nodes.CreateNode(ctx, &node, userID); err != nil {
		return model.Node{}, err
	}
	// Grant the creator full permission on the new timeseries.
	priv := model.NewNodePrivilege(node.ID, userID, userID)
	if err := s.privs.CreateNodePriv(ctx, &priv); err != nil {
		return model.Node{}, err
	}
	return node, nil
}

func (s *timeseriesService) Get(ctx context.Context, id int32, userID int32) (model.Node, error) {
	n, err := s.nodes.GetNode(ctx, id, userID)
	if err != nil {
		return model.Node{}, err
	}
	if n.Kind != Kind {
		return model.Node{}, ErrNotTimeseries
	}
	return n, nil
}

func (s *timeseriesService) Update(ctx context.Context, id int32, fields map[string]interface{}, userID int32) error {
	n, err := s.Get(ctx, id, userID)
	if err != nil {
		return err
	}
	_ = n
	return s.nodes.UpdateNode(ctx, id, &fields)
}

func (s *timeseriesService) Delete(ctx context.Context, id int32, userID int32) error {
	if _, err := s.Get(ctx, id, userID); err != nil {
		return err
	}
	// Remove the QuestDB data first so a failed node delete does not leave the
	// metadata in place referencing data that is gone. Deletion is not
	// transactional across the two stores; see plan notes.
	if err := s.queries.Delete(ctx, id); err != nil {
		return err
	}
	return s.nodes.DeleteNode(ctx, id)
}

func (s *timeseriesService) AddData(ctx context.Context, id int32, pts []DataPoint, userID int32) error {
	if _, err := s.Get(ctx, id, userID); err != nil {
		return err
	}
	return s.queries.Insert(ctx, id, pts)
}

func (s *timeseriesService) QueryData(ctx context.Context, id int32, q TimeseriesQuery, userID int32) ([]DataPoint, error) {
	if _, err := s.Get(ctx, id, userID); err != nil {
		return nil, err
	}
	return s.queries.Query(ctx, id, q.From, q.To, q.Agg, q.Limit)
}
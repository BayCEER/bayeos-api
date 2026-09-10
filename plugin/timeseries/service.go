package timeseries

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
)

// ErrNotTimeseries is returned when a node exists but is not a Timeseries.
var ErrNotTimeseries = errors.New("node is not a timeseries")

// TimeseriesService exposes the plugin's operations on top of the shared node
// tree and the QuestDB data repo.
type TimeseriesService interface {
	Create(ctx context.Context, cre TimeseriesCreate, baseNodeID int32, userID int32) (model.Node, error)
	Get(ctx context.Context, id int32, userID int32) (model.Node, error)
	Update(ctx context.Context, id int32, fields map[string]interface{}, userID int32) error
	Delete(ctx context.Context, id int32, userID int32) error

	AddData(ctx context.Context, id int32, pts []DataPoint, userID int32, ignoreInvalid bool) error
	AddDataBatch(ctx context.Context, series []SeriesData, userID int32, ignoreInvalid bool) error
	UpdateStatus(ctx context.Context, id int32, pts []StatusPoint, userID int32) error
	QueryData(ctx context.Context, id int32, q TimeseriesQuery, userID int32) (*TimeseriesData, error)
	QueryDataBatch(ctx context.Context, q TimeseriesDataQuery, userID int32) (*TimeseriesBatchData, error)
	GetStats(ctx context.Context, id int32, q TimeseriesStatsQuery, userID int32) (*TimeseriesStats, error)
}

type timeseriesService struct {
	nodes   service.NodeService
	privs   service.NodePrivService
	queries QuestRepo
}

// NewTimeseriesService returns a TimeseriesService backed by node/priv services
// and a QuestRepo for data.
func NewTimeseriesService(nodes service.NodeService, privs service.NodePrivService, queries QuestRepo) TimeseriesService {
	return &timeseriesService{nodes: nodes, privs: privs, queries: queries}
}

func (s *timeseriesService) Create(ctx context.Context, cre TimeseriesCreate, baseNodeID int32, userID int32) (model.Node, error) {
	node := model.Node{Kind: Kind}
	if cre.DisplayName != "" {
		node.DisplayName = cre.DisplayName
	}
	if cre.Metadata != nil {
		md, err := model.MetadataFromMap(cre.Metadata)
		if err != nil {
			return model.Node{}, fmt.Errorf("invalid metadata: %w", err)
		}
		node.Metadata = md
	}

	if cre.Path != nil && *cre.Path != "" {
		rel := *cre.Path
		if len(rel) > 0 && (rel[0] == '/' || rel[0] == '~') {
			rel = rel[1:]
		}
		if err := s.nodes.CreateNodeByPath(ctx, &node, baseNodeID, rel, userID); err != nil {
			return model.Node{}, err
		}
	} else {
		if cre.ParentID == nil {
			return model.Node{}, fmt.Errorf("either path or parent_id is required")
		}
		node.ParentID = *cre.ParentID
		if _, err := s.nodes.GetNode(ctx, node.ParentID, userID); err != nil {
			return model.Node{}, fmt.Errorf("parent node %d not found or not readable: %w", node.ParentID, err)
		}
		if err := s.nodes.CreateNode(ctx, &node, userID); err != nil {
			return model.Node{}, err
		}
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
	// QuestDB does not support row-level DELETE. Series data is orphaned when
	// the Postgres node is removed; storage is reclaimed via partition drops.
	return s.nodes.DeleteNode(ctx, id)
}

func (s *timeseriesService) AddData(ctx context.Context, id int32, pts []DataPoint, userID int32, ignoreInvalid bool) error {
	if _, err := s.Get(ctx, id, userID); err != nil {
		return err
	}
	if ignoreInvalid {
		pts = filterInvalidDataPoints(pts)
	}
	return s.queries.Insert(ctx, id, pts)
}

// AddDataBatch writes points for multiple series in a single batch. Every
// series id is validated first (it must exist, be a readable Timeseries and
// the caller must have permission); if any series fails the whole batch
// errors out and nothing is written.
func (s *timeseriesService) AddDataBatch(ctx context.Context, series []SeriesData, userID int32, ignoreInvalid bool) error {
	for _, srs := range series {
		if _, err := s.Get(ctx, srs.ID, userID); err != nil {
			return err
		}
	}
	if ignoreInvalid {
		series = filterInvalidSeries(series)
	}
	return s.queries.InsertBatch(ctx, series)
}

// isInvalidTimestamp reports whether a timestamp lies outside the accepted
// range: before the year 1970 or more than 100 years in the future.
func isInvalidTimestamp(t time.Time) bool {
	return t.Year() < 1970 || t.After(time.Now().AddDate(100, 0, 0))
}

// filterInvalidDataPoints drops data points whose timestamps are invalid.
func filterInvalidDataPoints(pts []DataPoint) []DataPoint {
	kept := make([]DataPoint, 0, len(pts))
	for _, p := range pts {
		if isInvalidTimestamp(p.Ts) {
			continue
		}
		kept = append(kept, p)
	}
	return kept
}

// filterInvalidSeries drops data points with invalid timestamps from each
// series, keeping the parallel ts/values arrays in lockstep.
func filterInvalidSeries(series []SeriesData) []SeriesData {
	out := make([]SeriesData, 0, len(series))
	for i := range series {
		s := series[i]
		keptTs := make([]time.Time, 0, len(s.Ts))
		keptVal := make([]float64, 0, len(s.Values))
		for j := range s.Ts {
			if isInvalidTimestamp(s.Ts[j]) {
				continue
			}
			keptTs = append(keptTs, s.Ts[j])
			keptVal = append(keptVal, s.Values[j])
		}
		s.Ts = keptTs
		s.Values = keptVal
		out = append(out, s)
	}
	return out
}

func (s *timeseriesService) UpdateStatus(ctx context.Context, id int32, pts []StatusPoint, userID int32) error {
	if _, err := s.Get(ctx, id, userID); err != nil {
		return err
	}
	return s.queries.UpsertStatusBatch(ctx, id, pts)
}

func (s *timeseriesService) QueryData(ctx context.Context, id int32, q TimeseriesQuery, userID int32) (*TimeseriesData, error) {
	if _, err := s.Get(ctx, id, userID); err != nil {
		return nil, err
	}
	tsType, err := normalizeTsType(q.TsType)
	if err != nil {
		return nil, err
	}
	sampleFunc, err := resolveSampleFunc(q.SampleInterval, q.SampleFunc)
	if err != nil {
		return nil, err
	}
	pts, err := s.queries.Query(ctx, id, q.From, q.To, q.SampleInterval, sampleFunc, q.Limit)
	if err != nil {
		return nil, err
	}

	ts := make([]time.Time, 0, len(pts))
	values := make([]float64, 0, len(pts))
	for _, p := range pts {
		ts = append(ts, p.Ts)
		values = append(values, p.Value)
	}
	series := []SeriesData{{ID: id, Ts: ts, Values: values}}
	return newTimeseriesData(q.SampleInterval, sampleFunc, series, tsType), nil
}

func (s *timeseriesService) QueryDataBatch(ctx context.Context, q TimeseriesDataQuery, userID int32) (*TimeseriesBatchData, error) {
	tsType, err := normalizeTsType(q.TsType)
	if err != nil {
		return nil, err
	}
	sampleFunc, err := resolveSampleFunc(q.SampleInterval, q.SampleFunc)
	if err != nil {
		return nil, err
	}

	// Only include timeseries the caller may read; unknown, non-timeseries and
	// non-readable ids are silently skipped rather than failing the whole batch.
	readable := make([]int32, 0, len(q.IDs))
	for _, id := range q.IDs {
		if _, err := s.Get(ctx, id, userID); err == nil {
			readable = append(readable, id)
		}
	}

	series, err := s.queries.QueryBatch(ctx, readable, q.From, q.To, q.SampleInterval, sampleFunc, q.Limit)
	if err != nil {
		return nil, err
	}
	data := &TimeseriesBatchData{Series: series, TsType: tsType}
	if q.SampleInterval != "" {
		data.SampleInterval = q.SampleInterval
		data.SampleFunc = sampleFunc
	}
	return data, nil
}

// normalizeTsType validates and normalizes the ts_type timestamp encoding
// parameter: empty or "iso" is the default (RFC3339 strings), "epoch" selects
// epoch microseconds, anything else is rejected.
func normalizeTsType(tsType string) (string, error) {
	if tsType == "" {
		tsType = "iso"
	}
	if tsType != "iso" && tsType != "epoch" {
		return "", fmt.Errorf("unsupported ts_type %q", tsType)
	}
	return tsType, nil
}

// resolveSampleFunc validates the sample_func rollup function. When the sample
// interval is empty (raw mode) the function is ignored and returns "". When an
// interval is set, empty or "avg" default to "avg"; anything else must be one
// of the supported rollup functions.
func resolveSampleFunc(interval, sampleFunc string) (string, error) {
	if interval == "" {
		return "", nil
	}
	if sampleFunc == "" {
		sampleFunc = "avg"
	}
	switch sampleFunc {
	case "avg", "count", "sum", "last", "min", "max", "first":
		return sampleFunc, nil
	default:
		return "", fmt.Errorf("unsupported sample_func %q", sampleFunc)
	}
}

// newTimeseriesData builds the single-series response. When the interval is
// empty (raw mode) the interval and function fields are omitted.
func newTimeseriesData(interval, sampleFunc string, series []SeriesData, tsType string) *TimeseriesData {
	data := &TimeseriesData{Series: series, TsType: tsType}
	if interval != "" {
		data.SampleInterval = interval
		data.SampleFunc = sampleFunc
	}
	return data
}

func (s *timeseriesService) GetStats(ctx context.Context, id int32, q TimeseriesStatsQuery, userID int32) (*TimeseriesStats, error) {
	n, err := s.Get(ctx, id, userID)
	if err != nil {
		return nil, err
	}

	stats, err := s.queries.ComputeStats(ctx, id, q.From, q.To, q.Agg)
	if err != nil {
		return nil, err
	}
	if stats.Count == 0 {
		return stats, nil
	}

	if resolution, ok := n.Metadata.Map()["resolution"]; ok {
		if resolutionSec, ok := resolution.(float64); ok && resolutionSec > 0 {
			complete, err := s.queries.ComputeCompleteness(ctx, id, int(resolutionSec), q.From, q.To)
			if err != nil {
				return nil, err
			}
			stats.Completeness = complete

			threshold := int(resolutionSec / 60 * 2)
			if threshold < 1 {
				threshold = 1
			}
			if gaps, err := s.queries.DetectGaps(ctx, id, threshold, q.From, q.To); err == nil {
				stats.Gaps = gaps
			}
		}
	}

	return stats, nil
}

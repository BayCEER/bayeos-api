package timeseries

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeQuestRepo is an in-memory QuestRepo used for unit tests without a live
// QuestDB.
type fakeQuestRepo struct {
	data              map[int32][]DataPoint
	status            map[int32][]StatusPoint
	lastInterval      string
	lastSampleFunc    string
	lastBatchSampleFn string
}

func newFakeQuestRepo() *fakeQuestRepo {
	return &fakeQuestRepo{data: map[int32][]DataPoint{}, status: map[int32][]StatusPoint{}}
}

func (f *fakeQuestRepo) Insert(ctx context.Context, nodeID int32, pts []DataPoint) error {
	f.data[nodeID] = append(f.data[nodeID], pts...)
	return nil
}

func (f *fakeQuestRepo) InsertBatch(ctx context.Context, series []SeriesData) error {
	for _, s := range series {
		for i := range s.Ts {
			f.data[s.ID] = append(f.data[s.ID], DataPoint{Ts: s.Ts[i], Value: s.Values[i]})
		}
	}
	return nil
}

func (f *fakeQuestRepo) UpsertStatusBatch(ctx context.Context, nodeID int32, pts []StatusPoint) error {
	f.status[nodeID] = append(f.status[nodeID], pts...)
	return nil
}

func (f *fakeQuestRepo) Query(ctx context.Context, nodeID int32, from, to *time.Time, interval string, sampleFunc string, limit *int) ([]DataPoint, error) {
	f.lastInterval = interval
	f.lastSampleFunc = sampleFunc
	return f.data[nodeID], nil
}

func (f *fakeQuestRepo) QueryBatch(ctx context.Context, ids []int32, from, to *time.Time, interval string, sampleFunc string, limit *int) ([]SeriesData, error) {
	f.lastInterval = interval
	f.lastBatchSampleFn = sampleFunc
	var out []SeriesData
	for _, id := range ids {
		s := SeriesData{ID: id}
		for _, p := range f.data[id] {
			s.Ts = append(s.Ts, p.Ts)
			s.Values = append(s.Values, p.Value)
		}
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeQuestRepo) ComputeStats(ctx context.Context, nodeID int32, from, to *time.Time, agg string) (*TimeseriesStats, error) {
	pts := f.data[nodeID]
	if len(pts) == 0 {
		return &TimeseriesStats{}, nil
	}
	stats := &TimeseriesStats{
		Count:   int64(len(pts)),
		FirstTs: pts[0].Ts,
		LastTs:  pts[len(pts)-1].Ts,
		Min:     pts[0].Value,
		Max:     pts[0].Value,
		Mean:    0,
	}
	for _, p := range pts {
		if p.Value < stats.Min {
			stats.Min = p.Value
		}
		if p.Value > stats.Max {
			stats.Max = p.Value
		}
		stats.Mean += p.Value
	}
	stats.Mean /= float64(len(pts))
	return stats, nil
}

func (f *fakeQuestRepo) ComputeCompleteness(ctx context.Context, nodeID int32, resolutionSec int, from, to *time.Time) (*CompletenessStats, error) {
	pts := f.data[nodeID]
	if len(pts) == 0 {
		return &CompletenessStats{}, nil
	}
	start := pts[0].Ts.Unix()
	end := pts[len(pts)-1].Ts.Unix()
	res := int64(resolutionSec)
	expected := end/res - start/res + 1
	actual := int64(len(pts))
	return &CompletenessStats{
		ExpectedCount:   expected,
		ActualCount:     actual,
		MissingCount:    expected - actual,
		CompletenessPct: float64(actual) * 100.0 / float64(expected),
	}, nil
}

func (f *fakeQuestRepo) DetectGaps(ctx context.Context, nodeID int32, thresholdMinutes int, from, to *time.Time) ([]GapInfo, error) {
	return nil, nil
}

func TestFakeQuestRepoRoundTrip(t *testing.T) {
	r := newFakeQuestRepo()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pts := []DataPoint{{Ts: ts, Value: 1.5}}

	require.NoError(t, r.Insert(context.Background(), 42, pts))
	got, err := r.Query(context.Background(), 42, nil, nil, "", "", nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, pts[0], got[0])
}

// stubNodeService is a minimal NodeService that returns a fixed Node and
// permission for tests.
type stubNodeService struct {
	node model.Node
}

func (s *stubNodeService) GetNode(ctx context.Context, id int32, userID int32) (model.Node, error) {
	return s.node, nil
}
func (s *stubNodeService) GetNodes(ctx context.Context, ids []int32, userID int32) ([]model.Node, error) {
	return []model.Node{s.node}, nil
}
func (s *stubNodeService) GetNodeChildren(ctx context.Context, id int32, userID int32) ([]model.Node, error) {
	return []model.Node{}, nil
}
func (s *stubNodeService) GetNodeParents(ctx context.Context, id int32, userID int32) ([]model.Node, error) {
	return []model.Node{}, nil
}
func (s *stubNodeService) CreateNode(ctx context.Context, node *model.Node, userID int32) error {
	return nil
}
func (s *stubNodeService) CreateNodeByPath(ctx context.Context, node *model.Node, baseNodeID int32, path string, userID int32) error {
	return nil
}
func (s *stubNodeService) UpdateNode(ctx context.Context, id int32, fields *map[string]interface{}) error {
	return nil
}
func (s *stubNodeService) UpdateNodeMetadata(ctx context.Context, id int32, metadata map[string]any) error {
	return nil
}
func (s *stubNodeService) DeleteNode(ctx context.Context, id int32) error {
	return nil
}
func (s *stubNodeService) FindNodeDescendantsByID(ctx context.Context, id int32) ([]int32, error) {
	return []int32{id}, nil
}
func (s *stubNodeService) DeleteNodesByIDs(ctx context.Context, ids []int32) error {
	return nil
}
func (s *stubNodeService) FindNodeByToken(ctx context.Context, token uuid.UUID) (model.Node, error) {
	return s.node, nil
}
func (s *stubNodeService) FindNodesByAntPath(ctx context.Context, pattern string, kind string, skip int, limit int, userID int32) (int32, []model.Node, error) {
	return 0, []model.Node{}, nil
}
func (s *stubNodeService) GetNodeTokens(ctx context.Context, id int32) ([]model.NodeToken, error) {
	return []model.NodeToken{}, nil
}
func (s *stubNodeService) GetNodePrivs(ctx context.Context, id int32) ([]model.NodePrivilege, error) {
	return []model.NodePrivilege{}, nil
}
func (s *stubNodeService) HasPermission(ctx context.Context, id int32, perm uint8, userID int32) (bool, error) {
	return true, nil
}

// stubNodePrivService is a minimal NodePrivService for tests.
type stubNodePrivService struct{}

func (s *stubNodePrivService) GetNodePriv(ctx context.Context, id int32) (model.NodePrivilege, error) {
	return model.NodePrivilege{}, nil
}
func (s *stubNodePrivService) CreateNodePriv(ctx context.Context, nodePriv *model.NodePrivilege) error {
	return nil
}
func (s *stubNodePrivService) DeleteNodePriv(ctx context.Context, id int32) error {
	return nil
}
func (s *stubNodePrivService) UpdateNodePriv(ctx context.Context, id int32, permission uint8) error {
	return nil
}
func (s *stubNodePrivService) GetNodePrivsByUserID(ctx context.Context, id int32) ([]model.NodePrivilege, error) {
	return []model.NodePrivilege{}, nil
}

func TestGetStatsWithResolution(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		r.data[7] = append(r.data[7], DataPoint{Ts: base.Add(time.Duration(i) * 3600 * time.Second), Value: float64(i)})
	}

	nodes := &stubNodeService{node: model.Node{
		ID:          7,
		DisplayName: "test",
		Kind:        Kind,
		ParentID:    1,
	}}
	var md model.Metadata
	md, err := model.MetadataFromMap(map[string]any{"resolution": float64(3600)})
	require.NoError(t, err)
	nodes.node.Metadata = md

	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)
	stats, err := svc.GetStats(context.Background(), 7, TimeseriesStatsQuery{}, 1)
	require.NoError(t, err)
	require.Equal(t, int64(10), stats.Count)
	require.Equal(t, float64(0), stats.Min)
	require.Equal(t, float64(9), stats.Max)
	require.NotNil(t, stats.Completeness)
	require.Equal(t, int64(10), stats.Completeness.ExpectedCount)
	require.Equal(t, int64(10), stats.Completeness.ActualCount)
	require.Equal(t, float64(100), stats.Completeness.CompletenessPct)
}

func TestGetStatsWithoutResolution(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		r.data[8] = append(r.data[8], DataPoint{Ts: base.Add(time.Duration(i) * 3600 * time.Second), Value: float64(i)})
	}

	nodes := &stubNodeService{node: model.Node{ID: 8, DisplayName: "test", Kind: Kind, ParentID: 1}}

	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)
	stats, err := svc.GetStats(context.Background(), 8, TimeseriesStatsQuery{}, 1)
	require.NoError(t, err)
	require.Equal(t, int64(5), stats.Count)
	require.Nil(t, stats.Completeness)
	require.Empty(t, stats.Gaps)
}

func TestQueryDataBatch(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}, {Ts: base.Add(time.Hour), Value: 2}}
	r.data[2] = []DataPoint{{Ts: base, Value: 10}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}

	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)
	data, err := svc.QueryDataBatch(context.Background(), TimeseriesDataQuery{IDs: []int32{1}}, 1)
	require.NoError(t, err)
	require.Equal(t, "iso", data.TsType)
	require.Len(t, data.Series, 1)
	require.Equal(t, int32(1), data.Series[0].ID)
	require.Len(t, data.Series[0].Ts, 2)
	require.Equal(t, []float64{1, 2}, data.Series[0].Values)
}

func TestQueryDataBatchTsTypeIso(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	data, err := svc.QueryDataBatch(context.Background(), TimeseriesDataQuery{IDs: []int32{1}, TsType: "iso"}, 1)
	require.NoError(t, err)

	raw, err := json.Marshal(data)
	require.NoError(t, err)

	var out struct {
		SampleInterval string `json:"sample_interval"`
		TsType         string `json:"ts_type"`
		Series         []struct {
			ID  int32     `json:"id"`
			Ts  []string  `json:"ts"`
			Val []float64 `json:"values"`
		} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "iso", out.TsType)
	require.Len(t, out.Series, 1)
	require.Equal(t, []string{base.Format(time.RFC3339Nano)}, out.Series[0].Ts)
}

func TestQueryDataBatchTsTypeUs(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	data, err := svc.QueryDataBatch(context.Background(), TimeseriesDataQuery{IDs: []int32{1}, TsType: "epoch"}, 1)
	require.NoError(t, err)

	raw, err := json.Marshal(data)
	require.NoError(t, err)

	var out struct {
		TsType string `json:"ts_type"`
		Series []struct {
			TS []int64   `json:"ts"`
			V  []float64 `json:"values"`
		} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "epoch", out.TsType)
	require.Len(t, out.Series, 1)
	require.Equal(t, []int64{base.UnixMicro()}, out.Series[0].TS)
}

func TestQueryDataBatchInvalidTsType(t *testing.T) {
	r := newFakeQuestRepo()
	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	_, err := svc.QueryDataBatch(context.Background(), TimeseriesDataQuery{IDs: []int32{1}, TsType: "bogus"}, 1)
	require.Error(t, err)
}

func TestQueryDataBatchSkipsNonTimeseries(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	// The stub returns a Folder kind for every requested id, so no id is a
	// readable timeseries and the batch must come back empty.
	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: "Folder", ParentID: 1}}

	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)
	data, err := svc.QueryDataBatch(context.Background(), TimeseriesDataQuery{IDs: []int32{1}}, 1)
	require.NoError(t, err)
	require.Empty(t, data.Series)
}

func TestQueryDataIsoDefault(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	data, err := svc.QueryData(context.Background(), 1, TimeseriesQuery{}, 1)
	require.NoError(t, err)
	require.Equal(t, "iso", data.TsType)
	require.Len(t, data.Series, 1)
	require.Equal(t, int32(1), data.Series[0].ID)
	require.Equal(t, []float64{1}, data.Series[0].Values)

	raw, err := json.Marshal(data)
	require.NoError(t, err)

	var out struct {
		TsType string `json:"ts_type"`
		Series []struct {
			ID  int32     `json:"id"`
			Ts  []string  `json:"ts"`
			Val []float64 `json:"values"`
		} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "iso", out.TsType)
	require.Len(t, out.Series, 1)
	require.Equal(t, int32(1), out.Series[0].ID)
	require.Equal(t, []string{base.Format(time.RFC3339Nano)}, out.Series[0].Ts)
	require.Equal(t, []float64{1}, out.Series[0].Val)
}

func TestQueryDataEpoch(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	data, err := svc.QueryData(context.Background(), 1, TimeseriesQuery{TsType: "epoch"}, 1)
	require.NoError(t, err)
	require.Equal(t, "epoch", data.TsType)
	require.Len(t, data.Series, 1)

	raw, err := json.Marshal(data)
	require.NoError(t, err)

	var out struct {
		TsType string `json:"ts_type"`
		Series []struct {
			ID  int32     `json:"id"`
			Ts  []int64   `json:"ts"`
			Val []float64 `json:"values"`
		} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "epoch", out.TsType)
	require.Len(t, out.Series, 1)
	require.Equal(t, int32(1), out.Series[0].ID)
	require.Equal(t, []int64{base.UnixMicro()}, out.Series[0].Ts)
	require.Equal(t, []float64{1}, out.Series[0].Val)
}

func TestQueryDataInvalidTsType(t *testing.T) {
	r := newFakeQuestRepo()
	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	_, err := svc.QueryData(context.Background(), 1, TimeseriesQuery{TsType: "bogus"}, 1)
	require.Error(t, err)
}

func TestQueryDataSampleFuncDefault(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	data, err := svc.QueryData(context.Background(), 1, TimeseriesQuery{SampleInterval: "1h"}, 1)
	require.NoError(t, err)
	require.Equal(t, "avg", r.lastSampleFunc)
	require.Equal(t, "1h", r.lastInterval)
	require.Equal(t, "1h", data.SampleInterval)
	require.Equal(t, "avg", data.SampleFunc)

	raw, err := json.Marshal(data)
	require.NoError(t, err)

	var out struct {
		SampleInterval string `json:"sample_interval"`
		SampleFunc     string `json:"sample_func"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "1h", out.SampleInterval)
	require.Equal(t, "avg", out.SampleFunc)
}

func TestQueryDataSampleFuncExplicit(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	data, err := svc.QueryData(context.Background(), 1, TimeseriesQuery{SampleInterval: "1d", SampleFunc: "last"}, 1)
	require.NoError(t, err)
	require.Equal(t, "last", r.lastSampleFunc)
	require.Equal(t, "1d", data.SampleInterval)
	require.Equal(t, "last", data.SampleFunc)
}

func TestQueryDataInvalidSampleFunc(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	_, err := svc.QueryData(context.Background(), 1, TimeseriesQuery{SampleInterval: "1h", SampleFunc: "bogus"}, 1)
	require.Error(t, err)
}

func TestQueryDataRawIgnoresSampleFunc(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	data, err := svc.QueryData(context.Background(), 1, TimeseriesQuery{SampleFunc: "bogus"}, 1)
	require.NoError(t, err)
	require.Empty(t, r.lastSampleFunc)
	require.Empty(t, data.SampleInterval)
	require.Empty(t, data.SampleFunc)
}

func TestQueryDataBatchSampleFunc(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.data[1] = []DataPoint{{Ts: base, Value: 1}}

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	data, err := svc.QueryDataBatch(context.Background(), TimeseriesDataQuery{IDs: []int32{1}, SampleInterval: "1M", SampleFunc: "sum"}, 1)
	require.NoError(t, err)
	require.Equal(t, "sum", r.lastBatchSampleFn)
	require.Equal(t, "1M", data.SampleInterval)
	require.Equal(t, "sum", data.SampleFunc)
}

func TestUpdateStatus(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	pts := []StatusPoint{
		{Ts: base, Value: 1, Status: 1},
		{Ts: base.Add(time.Hour), Value: 2, Status: 0},
	}
	require.NoError(t, svc.UpdateStatus(context.Background(), 1, pts, 1))
	require.Equal(t, pts, r.status[1])
}

func TestUpdateStatusNotTimeseries(t *testing.T) {
	r := newFakeQuestRepo()

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: "Folder", ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	err := svc.UpdateStatus(context.Background(), 1, []StatusPoint{{Ts: time.Now(), Value: 1, Status: 1}}, 1)
	require.ErrorIs(t, err, ErrNotTimeseries)
	require.Empty(t, r.status)
}

func TestAddDataBatch(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	series := []SeriesData{
		{ID: 1, Ts: []time.Time{base}, Values: []float64{1.5}},
		{ID: 2, Ts: []time.Time{base.Add(time.Hour)}, Values: []float64{2.5}},
	}
	// stubNodeService returns the same node (ID 1) for every id, so all series
	// pass the Timeseries check and are inserted.
	require.NoError(t, svc.AddDataBatch(context.Background(), series, 1, false))
	require.Len(t, r.data[1], 1)
	require.Equal(t, 1.5, r.data[1][0].Value)
	require.Len(t, r.data[2], 1)
	require.Equal(t, 2.5, r.data[2][0].Value)
}

func TestAddDataBatchNotTimeseriesFails(t *testing.T) {
	r := newFakeQuestRepo()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// The stub returns a Folder for every id, so no series is a Timeseries.
	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: "Folder", ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	series := []SeriesData{
		{ID: 1, Ts: []time.Time{base}, Values: []float64{1.5}},
	}
	err := svc.AddDataBatch(context.Background(), series, 1, false)
	require.ErrorIs(t, err, ErrNotTimeseries)
	require.Empty(t, r.data)
}

func TestAddDataIgnoreInvalidTimestamps(t *testing.T) {
	r := newFakeQuestRepo()
	valid := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	before1970 := time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC)
	farFuture := time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC)

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	pts := []DataPoint{
		{Ts: valid, Value: 1},
		{Ts: before1970, Value: 2},
		{Ts: farFuture, Value: 3},
	}
	// Default (ignoreInvalid=false) keeps every point.
	require.NoError(t, svc.AddData(context.Background(), 1, pts, 1, false))
	require.Len(t, r.data[1], 3)

	r.data[1] = nil
	require.NoError(t, svc.AddData(context.Background(), 1, pts, 1, true))
	require.Len(t, r.data[1], 1)
	require.Equal(t, valid, r.data[1][0].Ts)
	require.Equal(t, 1.0, r.data[1][0].Value)
}

func TestAddDataBatchIgnoreInvalidTimestamps(t *testing.T) {
	r := newFakeQuestRepo()
	valid := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	before1970 := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	farFuture := time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC)

	nodes := &stubNodeService{node: model.Node{ID: 1, DisplayName: "a", Kind: Kind, ParentID: 1}}
	svc := NewTimeseriesService(nodes, &stubNodePrivService{}, r)

	series := []SeriesData{
		{ID: 1, Ts: []time.Time{valid, before1970, farFuture}, Values: []float64{10, 20, 30}},
	}
	require.NoError(t, svc.AddDataBatch(context.Background(), series, 1, true))
	require.Len(t, r.data[1], 1)
	require.Equal(t, valid, r.data[1][0].Ts)
	require.Equal(t, 10.0, r.data[1][0].Value)
}

func TestDataPointUnmarshalISO(t *testing.T) {
	var dp DataPoint
	err := json.Unmarshal([]byte(`{"ts":"2026-01-01T12:30:00Z","value":3.14}`), &dp)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 1, 1, 12, 30, 0, 0, time.UTC), dp.Ts)
	require.Equal(t, 3.14, dp.Value)
}

func TestDataPointUnmarshalEpoch(t *testing.T) {
	var dp DataPoint
	epoch := int64(1735689600000000)
	err := json.Unmarshal([]byte(fmt.Sprintf(`{"ts":%d,"value":2.71}`, epoch)), &dp)
	require.NoError(t, err)
	require.Equal(t, time.UnixMicro(epoch), dp.Ts)
	require.Equal(t, 2.71, dp.Value)
}

func TestDataPointUnmarshalInvalid(t *testing.T) {
	var dp DataPoint
	err := json.Unmarshal([]byte(`{"ts":true,"value":1}`), &dp)
	require.Error(t, err)
}

func TestDataPointUnmarshalNull(t *testing.T) {
	var dp DataPoint
	err := json.Unmarshal([]byte(`{"ts":null,"value":1}`), &dp)
	require.Error(t, err)
}

func TestSeriesDataUnmarshalISO(t *testing.T) {
	var sd SeriesData
	raw := `{"id":5,"ts":["2026-01-01T00:00:00Z","2026-01-02T00:00:00Z"],"values":[10,20]}`
	require.NoError(t, json.Unmarshal([]byte(raw), &sd))
	require.Equal(t, int32(5), sd.ID)
	require.Len(t, sd.Ts, 2)
	require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), sd.Ts[0])
	require.Equal(t, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), sd.Ts[1])
	require.Equal(t, []float64{10, 20}, sd.Values)
}

func TestSeriesDataUnmarshalMixed(t *testing.T) {
	var sd SeriesData
	epoch := int64(1735689600000000)
	raw := fmt.Sprintf(`{"id":3,"ts":["2026-01-01T00:00:00Z",%d],"values":[1,2]}`, epoch)
	require.NoError(t, json.Unmarshal([]byte(raw), &sd))
	require.Len(t, sd.Ts, 2)
	require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), sd.Ts[0])
	require.Equal(t, time.UnixMicro(epoch), sd.Ts[1])
}

package timeseries

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeQuestRepo is an in-memory QuestRepo used for unit tests without a live
// QuestDB.
type fakeQuestRepo struct {
	data map[int32][]DataPoint
}

func newFakeQuestRepo() *fakeQuestRepo {
	return &fakeQuestRepo{data: map[int32][]DataPoint{}}
}

func (f *fakeQuestRepo) Insert(ctx context.Context, nodeID int32, pts []DataPoint) error {
	f.data[nodeID] = append(f.data[nodeID], pts...)
	return nil
}

func (f *fakeQuestRepo) Query(ctx context.Context, nodeID int32, from, to *time.Time, agg string, limit *int) ([]DataPoint, error) {
	return f.data[nodeID], nil
}

func (f *fakeQuestRepo) Delete(ctx context.Context, nodeID int32) error {
	delete(f.data, nodeID)
	return nil
}

func TestFakeQuestRepoRoundTrip(t *testing.T) {
	r := newFakeQuestRepo()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pts := []DataPoint{{Ts: ts, Value: 1.5}}

	require.NoError(t, r.Insert(context.Background(), 42, pts))
	got, err := r.Query(context.Background(), 42, nil, nil, "", nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, pts[0], got[0])

	require.NoError(t, r.Delete(context.Background(), 42))
	got, err = r.Query(context.Background(), 42, nil, nil, "", nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

package decorator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var base = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

// newTestSpan returns a span tree laid out as:
//
//	1 root
//	├── 2 folder
//	│   ├── 4 timeseries
//	│   └── 5 empty folder
//	└── 3 timeseries
func newTestSpan(t *testing.T) service.NodeTimeSpanServiceInterface {
	t.Helper()
	ts, err := service.InitializeTree([]model.NodeTimeSpan{
		{ID: 1, ParentID: 0},
		{ID: 2, ParentID: 1},
		{ID: 3, ParentID: 1, LocalFirst: base.Add(2 * time.Hour), LocalLast: base.Add(3 * time.Hour)},
		{ID: 4, ParentID: 2, LocalFirst: base.Add(1 * time.Hour), LocalLast: base.Add(4 * time.Hour)},
		{ID: 5, ParentID: 2},
	})
	require.NoError(t, err)
	return ts
}

// enrichVia runs the decorator's Enrich function over a single node and returns
// the result, without going through Chain, so the test targets the enricher
// alone.
func enrichVia(t *testing.T, d Decorator, n model.Node) model.Node {
	t.Helper()
	require.NotNil(t, d.Enrich, "decorator must contribute an enrich function")

	v := NewView(&n)
	if matches(d.When, v) {
		d.Enrich(context.Background(), v)
		v.Commit()
	}
	return *v.Node
}

func TestTimeSpanEnrich(t *testing.T) {
	t.Run("writes the aggregated span as RFC3339", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{Span: newTestSpan(t)})

		n := enrichVia(t, d, newNode(3, "Timeseries", nil))
		md := mdOf(n)

		assert.Equal(t, "2023-01-01T02:00:00Z", md[DefaultFirstKey])
		assert.Equal(t, "2023-01-01T03:00:00Z", md[DefaultLastKey])
	})

	t.Run("a folder receives the span rolled up from its subtree", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{Span: newTestSpan(t), Kinds: []string{"Folder", "Timeseries"}})

		md := mdOf(enrichVia(t, d, newNode(2, "Folder", nil)))

		assert.Equal(t, "2023-01-01T01:00:00Z", md[DefaultFirstKey])
		assert.Equal(t, "2023-01-01T04:00:00Z", md[DefaultLastKey])
	})

	t.Run("preserves existing metadata", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{Span: newTestSpan(t)})

		md := mdOf(enrichVia(t, d, newNode(3, "Timeseries", map[string]any{"unit": "C"})))

		assert.Equal(t, "C", md["unit"])
		assert.Equal(t, "2023-01-01T02:00:00Z", md[DefaultFirstKey])
	})

	t.Run("an unknown node is left alone", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{Span: newTestSpan(t)})

		md := mdOf(enrichVia(t, d, newNode(99, "Timeseries", map[string]any{"unit": "C"})))

		assert.Equal(t, map[string]any{"unit": "C"}, md)
	})

	t.Run("OmitWhenEmpty skips a node with no data", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{Span: newTestSpan(t), OmitWhenEmpty: true})

		// Node 5 exists in the tree but neither it nor its subtree carries a
		// measurement.
		md := mdOf(enrichVia(t, d, newNode(5, "Folder", map[string]any{"unit": "C"})))
		assert.Equal(t, map[string]any{"unit": "C"}, md)
	})

	t.Run("without OmitWhenEmpty a zero span is still written", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{Span: newTestSpan(t)})

		md := mdOf(enrichVia(t, d, newNode(5, "Folder", nil)))

		// A zero bound renders as an empty string, not a year-one timestamp.
		assert.Equal(t, "", md[DefaultFirstKey])
		assert.Equal(t, "", md[DefaultLastKey])
	})

	t.Run("a node of an unlisted kind is not decorated", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{Span: newTestSpan(t), Kinds: []string{"Timeseries"}})

		md := mdOf(enrichVia(t, d, newNode(2, "Folder", nil)))
		assert.Empty(t, md)
	})

	t.Run("custom metadata keys", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{
			Span:     newTestSpan(t),
			FirstKey: "ts_from",
			LastKey:  "ts_to",
		})

		md := mdOf(enrichVia(t, d, newNode(3, "Timeseries", nil)))

		assert.Equal(t, "2023-01-01T02:00:00Z", md["ts_from"])
		assert.Equal(t, "2023-01-01T03:00:00Z", md["ts_to"])
		assert.NotContains(t, md, DefaultFirstKey)
	})

	t.Run("non-UTC input is normalised to UTC", func(t *testing.T) {
		loc := time.FixedZone("plus2", 2*60*60)
		ts, err := service.InitializeTree([]model.NodeTimeSpan{
			{ID: 1, ParentID: 0, LocalFirst: base.In(loc), LocalLast: base.Add(time.Hour).In(loc)},
		})
		require.NoError(t, err)

		d := TimeSpan(TimeSpanConfig{Span: ts})
		md := mdOf(enrichVia(t, d, newNode(1, "Timeseries", nil)))

		assert.Equal(t, "2023-01-01T00:00:00Z", md[DefaultFirstKey])
		assert.Equal(t, "2023-01-01T01:00:00Z", md[DefaultLastKey])
	})
}

func TestTimeSpanWrites(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateNode adds the node and re-aggregates", func(t *testing.T) {
		span := newTestSpan(t)
		base2 := &stubNode{node: newNode(7, "Timeseries", nil)}

		svc := Chain(base2, TimeSpan(TimeSpanConfig{Span: span, SyncWrites: true}))

		n := newNode(7, "Timeseries", nil)
		require.NoError(t, svc.CreateNode(ctx, &n, 1))

		// The base service assigned the id the decorator then registered.
		first, _, ok := span.GetTimeSpan(7)
		assert.True(t, ok)
		assert.True(t, first.IsZero())
	})

	t.Run("a failed CreateNode leaves the tree untouched", func(t *testing.T) {
		span := newTestSpan(t)
		base2 := &stubNode{err: assert.AnError}

		svc := Chain(base2, TimeSpan(TimeSpanConfig{Span: span, SyncWrites: true}))

		n := newNode(7, "Timeseries", nil)
		require.Error(t, svc.CreateNode(ctx, &n, 1))

		_, _, ok := span.GetTimeSpan(7)
		assert.False(t, ok, "a node that was never persisted must not enter the tree")
	})

	t.Run("UpdateNode with parent_id moves the node", func(t *testing.T) {
		span := newTestSpan(t)
		base2 := &stubNode{}

		svc := Chain(base2, TimeSpan(TimeSpanConfig{Span: span, SyncWrites: true}))

		// A JSON body decodes parent_id to float64; the node 4 timeseries is
		// moved out of folder 2 and under timeseries 3.
		fields := map[string]any{"parent_id": float64(3)}
		require.NoError(t, svc.UpdateNode(ctx, 4, &fields))

		// The base service saw the decoded body unchanged.
		assert.Equal(t, float64(3), base2.lastFields["parent_id"])
		// Node 4 is no longer rolled up into node 2, so only node 5 remains
		// there and contributes nothing.
		first, last, _ := span.GetTimeSpan(2)
		assert.True(t, first.IsZero())
		assert.True(t, last.IsZero())

		// It now rolls up into node 3 instead.
		first, last, _ = span.GetTimeSpan(3)
		assert.Equal(t, base.Add(time.Hour), first.UTC())
		assert.Equal(t, base.Add(4*time.Hour), last.UTC())
	})

	t.Run("UpdateNode without parent_id does not move the node", func(t *testing.T) {
		span := newTestSpan(t)
		base2 := &stubNode{}

		svc := Chain(base2, TimeSpan(TimeSpanConfig{Span: span, SyncWrites: true}))

		fields := map[string]any{"display_name": "renamed"}
		require.NoError(t, svc.UpdateNode(ctx, 4, &fields))

		first, _, _ := span.GetTimeSpan(2)
		assert.Equal(t, base.Add(time.Hour), first.UTC(), "the subtree must still roll up")
	})

	t.Run("DeleteNodesByIDs removes the whole subtree", func(t *testing.T) {
		span := newTestSpan(t)
		base2 := &stubNode{}

		svc := Chain(base2, TimeSpan(TimeSpanConfig{Span: span, SyncWrites: true}))
		require.NoError(t, svc.DeleteNodesByIDs(ctx, []int32{2}))

		_, _, ok := span.GetTimeSpan(2)
		assert.False(t, ok)
		_, _, ok = span.GetTimeSpan(4)
		assert.False(t, ok, "the descendant must be removed too")

		// The surviving branch still aggregates into the root.
		first, last, _ := span.GetTimeSpan(1)
		assert.Equal(t, base.Add(2*time.Hour), first.UTC())
		assert.Equal(t, base.Add(3*time.Hour), last.UTC())
	})

	t.Run("SyncWrites off leaves the tree alone", func(t *testing.T) {
		span := newTestSpan(t)
		base2 := &stubNode{}

		svc := Chain(base2, TimeSpan(TimeSpanConfig{Span: span}))
		require.NoError(t, svc.DeleteNodesByIDs(ctx, []int32{2}))

		_, _, ok := span.GetTimeSpan(2)
		assert.True(t, ok, "without SyncWrites the decorator is read-only")
	})
}

func TestTimeSpanConfig(t *testing.T) {
	t.Run("requires a span", func(t *testing.T) {
		assert.Panics(t, func() { TimeSpan(TimeSpanConfig{}) })
	})

	t.Run("a refresh interval requires a loader", func(t *testing.T) {
		assert.Panics(t, func() {
			TimeSpan(TimeSpanConfig{Span: newTestSpan(t), RefreshEvery: time.Minute})
		})
	})

	t.Run("names itself and defaults its keys", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{Span: newTestSpan(t)})
		assert.Equal(t, "timespan", d.Name)
		assert.NotNil(t, d.Enrich)
		assert.Nil(t, d.Wrap, "SyncWrites off means no wrapper")
	})

	t.Run("installs a wrapper when SyncWrites is on", func(t *testing.T) {
		d := TimeSpan(TimeSpanConfig{Span: newTestSpan(t), SyncWrites: true})
		assert.NotNil(t, d.Wrap)
	})
}

func TestToInt32(t *testing.T) {
	for _, v := range []any{int32(5), int(5), int64(5), float64(5), float32(5)} {
		got, ok := toInt32(v)
		require.True(t, ok, "%T", v)
		assert.Equal(t, int32(5), got)
	}

	_, ok := toInt32("5")
	assert.False(t, ok, "a non-numeric parent_id is not a move")
}

// countingSpan forwards to a real span tree and records how the decorator drove
// its delete path. The node handler hands DeleteNodesByIDs the full descendant
// set, so entering the span service once per id would mean one whole-tree
// rebuild per id; this makes that regression a test failure rather than
// something only a profile would reveal.
type countingSpan struct {
	service.NodeTimeSpanServiceInterface

	mu          sync.Mutex
	deleteNodes [][]int32
	deleteNode  []int32
}

func (c *countingSpan) DeleteNodes(nodeIDs []int32) {
	c.mu.Lock()
	c.deleteNodes = append(c.deleteNodes, append([]int32(nil), nodeIDs...))
	c.mu.Unlock()
	c.NodeTimeSpanServiceInterface.DeleteNodes(nodeIDs)
}

func (c *countingSpan) DeleteNode(nodeID int32) {
	c.mu.Lock()
	c.deleteNode = append(c.deleteNode, nodeID)
	c.mu.Unlock()
	c.NodeTimeSpanServiceInterface.DeleteNode(nodeID)
}

func TestTimeSpanDeleteIsBatched(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteNodesByIDs enters the span service once", func(t *testing.T) {
		span := &countingSpan{NodeTimeSpanServiceInterface: newTestSpan(t)}
		base2 := &stubNode{}

		svc := Chain(base2, TimeSpan(TimeSpanConfig{Span: span, SyncWrites: true}))

		// What the node handler passes: the whole descendant set, not just the
		// root of the removed subtree.
		ids := []int32{2, 4, 5}
		require.NoError(t, svc.DeleteNodesByIDs(ctx, ids))

		require.Len(t, span.deleteNodes, 1, "one rebuild, not one per id")
		assert.Equal(t, ids, span.deleteNodes[0])
		assert.Empty(t, span.deleteNode, "the singular delete must not be used on this path")

		// The ids reached the base service unchanged.
		assert.Equal(t, ids, base2.lastDeleteIDs)

		// The tree is left consistent: only the 3 branch survives.
		remaining := span.GetNodes()
		require.Len(t, remaining, 2)
		first, last, _ := span.GetTimeSpan(1)
		assert.Equal(t, base.Add(2*time.Hour), first.UTC())
		assert.Equal(t, base.Add(3*time.Hour), last.UTC())
	})

	t.Run("the singular delete still works", func(t *testing.T) {
		span := &countingSpan{NodeTimeSpanServiceInterface: newTestSpan(t)}
		base2 := &stubNode{}

		svc := Chain(base2, TimeSpan(TimeSpanConfig{Span: span, SyncWrites: true}))
		require.NoError(t, svc.DeleteNode(ctx, 2))

		assert.Equal(t, []int32{2}, span.deleteNode)
		assert.Empty(t, span.deleteNodes)

		_, _, ok := span.GetTimeSpan(4)
		assert.False(t, ok, "the cascade must reach descendants")
	})
}

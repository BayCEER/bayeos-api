package decorator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncSpan(t *testing.T) {
	t.Run("delegates to the current tree", func(t *testing.T) {
		first := newTestSpan(t)
		holder := &syncSpan{impl: first}

		f, l, ok := holder.GetTimeSpan(3)
		require.True(t, ok)
		assert.Equal(t, base.Add(2*time.Hour), f.UTC())
		assert.Equal(t, base.Add(3*time.Hour), l.UTC())
	})

	t.Run("a swap is visible to subsequent reads", func(t *testing.T) {
		holder := &syncSpan{impl: newTestSpan(t)}

		replacement, err := service.InitializeTree([]model.NodeTimeSpan{
			{ID: 42, ParentID: 0, LocalFirst: base, LocalLast: base.Add(time.Hour)},
		})
		require.NoError(t, err)

		holder.set(replacement)

		_, _, ok := holder.GetTimeSpan(3)
		assert.False(t, ok, "the previous tree is gone")
		f, _, ok := holder.GetTimeSpan(42)
		require.True(t, ok)
		assert.Equal(t, base, f.UTC())
	})

	t.Run("forwards mutations to the current tree", func(t *testing.T) {
		span := newTestSpan(t)
		holder := &syncSpan{impl: span}

		holder.AddNode(7, 2)
		holder.SetLocalMetrics(7, base.Add(5*time.Hour), base.Add(6*time.Hour))
		holder.MoveNode(7, 3)
		holder.DeleteNode(7)
		holder.AddNodes([]model.NodeTimeSpan{{ID: 8, ParentID: 1}})

		assert.Len(t, holder.GetNodes(), 6)
	})
}

func TestBuildSpanTree(t *testing.T) {
	ctx := context.Background()

	t.Run("aggregates the loaded nodes", func(t *testing.T) {
		ts, err := buildSpanTree(ctx, func(context.Context) ([]model.NodeTimeSpan, error) {
			return []model.NodeTimeSpan{
				{ID: 1, ParentID: 0},
				{ID: 2, ParentID: 1, LocalFirst: base.Add(time.Hour), LocalLast: base.Add(2 * time.Hour)},
			}, nil
		})
		require.NoError(t, err)

		f, l, ok := ts.GetTimeSpan(1)
		require.True(t, ok)
		assert.Equal(t, base.Add(time.Hour), f.UTC())
		assert.Equal(t, base.Add(2*time.Hour), l.UTC())
	})

	t.Run("propagates a loader failure", func(t *testing.T) {
		_, err := buildSpanTree(ctx, func(context.Context) ([]model.NodeTimeSpan, error) {
			return nil, errors.New("questdb unreachable")
		})
		require.Error(t, err)
	})
}

func TestTimeSpanRefresh(t *testing.T) {
	t.Run("the refresher replaces the tree and the enricher sees it", func(t *testing.T) {
		var calls atomic.Int32
		loader := func(context.Context) ([]model.NodeTimeSpan, error) {
			calls.Add(1)
			return []model.NodeTimeSpan{
				{ID: 1, ParentID: 0, LocalFirst: base, LocalLast: base.Add(time.Hour)},
				{ID: 9, ParentID: 1, LocalFirst: base.Add(2 * time.Hour), LocalLast: base.Add(3 * time.Hour)},
			}, nil
		}

		// The initial tree has node 1 rolling up to base+1h; the loader's tree
		// has it at base. The switch between the two is what proves the swap
		// reached the enricher.
		d := TimeSpan(TimeSpanConfig{
			Span:         newTestSpan(t),
			Loader:       loader,
			RefreshEvery: 5 * time.Millisecond,
		})

		first := func() string {
			n := enrichVia(t, d, newNode(1, "Timeseries", nil))
			v, _ := mdOf(n)[DefaultFirstKey].(string)
			return v
		}

		require.Equal(t, "2023-01-01T01:00:00Z", first(), "the initial tree is in use")

		require.Eventually(t, func() bool {
			return first() == "2023-01-01T00:00:00Z"
		}, 2*time.Second, 5*time.Millisecond, "the refresher should swap the tree")

		assert.Positive(t, calls.Load())

		// A node that only exists in the loaded tree is now decorated too,
		// which is how a node created outside the decorated service becomes
		// visible.
		n := enrichVia(t, d, newNode(9, "Timeseries", nil))
		assert.Equal(t, "2023-01-01T02:00:00Z", mdOf(n)[DefaultFirstKey])
	})
}

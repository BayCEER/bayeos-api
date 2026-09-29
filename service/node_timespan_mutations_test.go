package service

import (
	"testing"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spanBase is the origin the times in these tests are expressed against.
var spanBase = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

// spanOf reads a node's aggregated bounds, failing if the node is absent.
func spanOf(t *testing.T, ts *NodeTimeSpanService, id int32) (time.Time, time.Time) {
	t.Helper()
	first, last, ok := ts.GetTimeSpan(id)
	require.True(t, ok, "node %d must exist", id)
	return first, last
}

func mustTree(t *testing.T, nodes []model.NodeTimeSpan) *NodeTimeSpanService {
	t.Helper()
	ts, err := InitializeTree(nodes)
	require.NoError(t, err)
	return ts
}

// TestMutationsKeepAggregatesValid pins the contract that makes the span service
// safe to use: every mutator leaves the tree internally consistent.
//
// Re-aggregation used to be a separate public step that callers had to remember.
// No mutation test asserted a time span afterwards, and a stale bound only ever
// errs outwards (an early first, a late last), so it never corrected itself.
func TestMutationsKeepAggregatesValid(t *testing.T) {
	t.Run("AddNode of an empty node leaves aggregates valid", func(t *testing.T) {
		ts := mustTree(t, createTestNodesWithTimes())
		beforeFirst, beforeLast := spanOf(t, ts, 1)

		ts.AddNode(7, 3)

		// A node with no measurements of its own contributes nothing to the
		// roll-up, and no existing node changed parent, so no sweep is needed.
		first, last := spanOf(t, ts, 1)
		assert.Equal(t, beforeFirst, first)
		assert.Equal(t, beforeLast, last)

		first, last = spanOf(t, ts, 7)
		assert.True(t, first.IsZero())
		assert.True(t, last.IsZero())
	})

	t.Run("AddNodes rolls populated bounds up to the root", func(t *testing.T) {
		ts := mustTree(t, createTestNodesWithTimes())

		// Deliberately outside the existing range at both ends.
		ts.AddNodes([]model.NodeTimeSpan{{
			ID:         7,
			ParentID:   3,
			LocalFirst: spanBase.Add(-1 * time.Hour),
			LocalLast:  spanBase.Add(7 * time.Hour),
		}})

		first, last := spanOf(t, ts, 1)
		assert.Equal(t, spanBase.Add(-1*time.Hour), first, "the root must pick up the new first")
		assert.Equal(t, spanBase.Add(7*time.Hour), last, "the root must pick up the new last")

		first, last = spanOf(t, ts, 3)
		assert.Equal(t, spanBase.Add(-1*time.Hour), first)
		assert.Equal(t, spanBase.Add(7*time.Hour), last)
	})

	t.Run("SetLocalMetrics propagates up the chain", func(t *testing.T) {
		ts := mustTree(t, createTestNodesWithTimes())

		ts.SetLocalMetrics(4, spanBase.Add(10*time.Hour), spanBase.Add(11*time.Hour))

		// Node 2 keeps its own earlier first but adopts node 4's later last.
		first, last := spanOf(t, ts, 2)
		assert.Equal(t, spanBase.Add(1*time.Hour), first)
		assert.Equal(t, spanBase.Add(11*time.Hour), last)

		// The root picks up the new maximum too.
		_, last = spanOf(t, ts, 1)
		assert.Equal(t, spanBase.Add(11*time.Hour), last)
	})

	t.Run("MoveNode transfers bounds between branches", func(t *testing.T) {
		ts := mustTree(t, createTestNodesWithTimes())

		// Node 2 carries 01:00-05:00 through its children; folding it under
		// node 3 must pull that branch's first earlier than it was.
		ts.MoveNode(2, 3)

		first, last := spanOf(t, ts, 3)
		assert.Equal(t, spanBase.Add(1*time.Hour), first, "node 2's first must now roll into node 3")
		assert.Equal(t, spanBase.Add(6*time.Hour), last)

		// Node 6 keeps its own bounds; it is still a direct child of node 3.
		first, last = spanOf(t, ts, 6)
		assert.Equal(t, spanBase.Add(5*time.Hour), first)
		assert.Equal(t, spanBase.Add(6*time.Hour), last)
	})

	t.Run("MoveNode to root detaches the subtree", func(t *testing.T) {
		// ParentID 0 means "root", so this promotes node 2 to a sibling of node
		// 1 rather than detaching it from the tree. The root must stop counting
		// it, which is only observable when the root has no bounds of its own.
		ts := mustTree(t, []model.NodeTimeSpan{
			{ID: 1, ParentID: 0}, // root, no data of its own
			{ID: 2, ParentID: 1, LocalFirst: spanBase.Add(1 * time.Hour), LocalLast: spanBase.Add(5 * time.Hour)},
			{ID: 3, ParentID: 1, LocalFirst: spanBase.Add(2 * time.Hour), LocalLast: spanBase.Add(3 * time.Hour)},
		})

		ts.MoveNode(2, 0)

		// Only the node 3 series is left under the root.
		first, last := spanOf(t, ts, 1)
		assert.Equal(t, spanBase.Add(2*time.Hour), first)
		assert.Equal(t, spanBase.Add(3*time.Hour), last)

		// The promoted node still aggregates itself.
		first, last = spanOf(t, ts, 2)
		assert.Equal(t, spanBase.Add(1*time.Hour), first)
		assert.Equal(t, spanBase.Add(5*time.Hour), last)
	})

	// The case that motivated folding the re-aggregation into the mutators: a
	// stale bound can only err outwards, so after removing the branch that held
	// the extremes the root's first has to move forward and its last backward.
	t.Run("DeleteNode moves a stale bound back inwards", func(t *testing.T) {
		ts := mustTree(t, []model.NodeTimeSpan{
			{ID: 1, ParentID: 0}, // root, no data of its own
			{ID: 2, ParentID: 1}, // folder, to be deleted
			{ID: 3, ParentID: 2, LocalFirst: spanBase.Add(1 * time.Hour), LocalLast: spanBase.Add(5 * time.Hour)},
			{ID: 4, ParentID: 1, LocalFirst: spanBase.Add(2 * time.Hour), LocalLast: spanBase.Add(3 * time.Hour)},
		})

		first, last := spanOf(t, ts, 1)
		require.Equal(t, spanBase.Add(1*time.Hour), first)
		require.Equal(t, spanBase.Add(5*time.Hour), last)

		ts.DeleteNode(2)

		// Only the node 4 series is left, so the root must report its range
		// exactly. Without a re-aggregation the root would still claim
		// 01:00-05:00 from the deleted branch.
		first, last = spanOf(t, ts, 1)
		assert.Equal(t, spanBase.Add(2*time.Hour), first)
		assert.Equal(t, spanBase.Add(3*time.Hour), last)
	})

	t.Run("deleting the last data node empties the root", func(t *testing.T) {
		ts := mustTree(t, []model.NodeTimeSpan{
			{ID: 1, ParentID: 0},
			{ID: 2, ParentID: 1, LocalFirst: spanBase, LocalLast: spanBase.Add(time.Hour)},
		})

		ts.DeleteNode(2)

		first, last := spanOf(t, ts, 1)
		assert.True(t, first.IsZero())
		assert.True(t, last.IsZero())
	})
}

func TestDeleteNodes(t *testing.T) {
	t.Run("removes several disjoint subtrees in one call", func(t *testing.T) {
		ts := mustTree(t, createTestNodes())

		ts.DeleteNodes([]int32{2, 3})

		// 2 took 4 and 5 with it, 3 took 6; only the root survives.
		require.Len(t, ts.nodes, 1)
		assert.Equal(t, int32(1), ts.nodes[0].ID)
	})

	t.Run("a root and its descendant in the same batch is not double counted", func(t *testing.T) {
		ts := mustTree(t, createTestNodes())

		// Naming both 2 and its child 4 must remove exactly the same set as
		// naming 2 alone: 1, 3 and 6 survive.
		ts.DeleteNodes([]int32{2, 4})

		require.Len(t, ts.nodes, 3)
		for _, id := range []int32{2, 4, 5} {
			_, ok := ts.indexMap[id]
			assert.False(t, ok, "node %d must be gone", id)
		}
	})

	t.Run("cascades transitively to grandchildren", func(t *testing.T) {
		ts := mustTree(t, createTestNodes())

		// 6 is a grandchild of the root via 3.
		ts.DeleteNodes([]int32{3})

		for _, id := range []int32{3, 6} {
			_, ok := ts.indexMap[id]
			assert.False(t, ok, "node %d must be gone", id)
		}
		require.Len(t, ts.nodes, 4)
	})

	t.Run("ids that no longer exist are ignored", func(t *testing.T) {
		ts := mustTree(t, createTestNodes())
		before := len(ts.nodes)

		ts.DeleteNodes([]int32{99, 100})

		assert.Equal(t, before, len(ts.nodes), "an unknown id must not touch the tree")
	})

	t.Run("an empty batch is a no-op", func(t *testing.T) {
		ts := mustTree(t, createTestNodes())
		before := len(ts.nodes)

		ts.DeleteNodes(nil)
		ts.DeleteNodes([]int32{})

		assert.Equal(t, before, len(ts.nodes))
	})

	t.Run("the root can be removed together with a descendant", func(t *testing.T) {
		ts := mustTree(t, createTestNodes())

		ts.DeleteNodes([]int32{1, 4})

		assert.Empty(t, ts.nodes)
		assert.Empty(t, ts.indexMap)
	})

	t.Run("re-aggregates once for the whole batch", func(t *testing.T) {
		ts := mustTree(t, []model.NodeTimeSpan{
			{ID: 1, ParentID: 0},
			{ID: 2, ParentID: 1, LocalFirst: spanBase.Add(1 * time.Hour), LocalLast: spanBase.Add(5 * time.Hour)},
			{ID: 3, ParentID: 1, LocalFirst: spanBase.Add(2 * time.Hour), LocalLast: spanBase.Add(3 * time.Hour)},
		})

		ts.DeleteNodes([]int32{2})

		first, last := spanOf(t, ts, 1)
		assert.Equal(t, spanBase.Add(2*time.Hour), first)
		assert.Equal(t, spanBase.Add(3*time.Hour), last)
	})

	t.Run("surviving orphans are kept, not dropped", func(t *testing.T) {
		ts := mustTree(t, []model.NodeTimeSpan{
			{ID: 1, ParentID: 0},
			{ID: 2, ParentID: 1},
			{ID: 3, ParentID: 99, LocalFirst: spanBase, LocalLast: spanBase.Add(time.Hour)},
		})

		ts.DeleteNodes([]int32{2})

		require.Len(t, ts.nodes, 2)
		// The orphan keeps its own bounds but rolls up nowhere.
		first, last := spanOf(t, ts, 3)
		assert.Equal(t, spanBase, first)
		assert.Equal(t, spanBase.Add(time.Hour), last)
	})

	t.Run("the ordering invariant survives the cascade", func(t *testing.T) {
		ts := mustTree(t, createTestNodes())
		requireParentBeforeChild(t, ts)

		ts.DeleteNodes([]int32{2, 3})

		requireParentBeforeChild(t, ts)
	})
}

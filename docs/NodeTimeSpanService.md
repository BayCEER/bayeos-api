# High-Performance In-Memory Time Span Tree

`service.NodeTimeSpanService` holds one time span per node for the whole node tree
— roughly 50,000 nodes — in a single flat slice. It is optimised for bulk
structural changes and for aggregating `min(first)` / `max(last)` up the tree.

It is consumed by the `TimeSpan` decorator, which projects the result into node
metadata; see [NodeDecorator.md](NodeDecorator.md).

**Authoritative source:** [`service/node_timespan_service.go`](../service/node_timespan_service.go).
This document explains the design and the invariants. It deliberately does not
mirror the full source — a copy of the file would drift the moment the code
changes.

---

## Architecture highlights

- **Cache locality (arena allocation).** All nodes live in one flat
  `[]model.NodeTimeSpan`. The garbage collector sees a single allocation rather
  than 50,000 individual pointers.
- **Topological ordering.** Nodes are ordered breadth-first so that a child
  **always** appears after its parent. Two algorithms depend on this: the backward
  aggregation sweep, and the single-pass delete cascade.
- **Aggregation sweep in under a millisecond.** `min(first)` and `max(last)` are
  computed with one linear backwards loop, no recursion and no map lookups.
- **Frictionless mutex.** A `sync.RWMutex` lets reads run in parallel; only
  structural changes and batch updates take the write lock.
- **Self-consistent by construction.** Every mutator leaves aggregates correct for
  the current topology. There is no separate "recompute" step for callers to
  remember.

## Data layout

```go
type NodeTimeSpan struct {
	ID        int32
	ParentID  int32
	ParentIdx int32 // direct index in the array, for O(1) access without a map lookup

	LocalFirst time.Time // bounds measured on this node itself
	LocalLast  time.Time

	AggFirst time.Time // bounds of this node's whole subtree
	AggLast  time.Time
}
```

`ParentID` is the durable relationship from the database. `ParentIdx` is the
derived in-memory form: a parent is always at a **lower** index than its children,
so the roll-up is a pointer walk and never a map lookup. `ParentIdx` is `-1` for
roots and for orphans.

## The interface, and the invariant that defines it

```go
type NodeTimeSpanServiceInterface interface {
	SetLocalMetrics(nodeID int32, first, last time.Time)
	AddNode(nodeID int32, parentID int32)
	AddNodes(nodes []model.NodeTimeSpan)
	MoveNode(nodeID int32, newParentID int32)
	DeleteNode(nodeID int32)
	DeleteNodes(nodeIDs []int32)
	GetTimeSpan(nodeID int32) (first time.Time, last time.Time, exists bool)
	GetNodes() []model.NodeTimeSpan
}
```

**Every method leaves the tree internally consistent.** `GetTimeSpan` can never
return a stale aggregate, and callers never repair anything after a mutation.

### Which mutations re-aggregate, and why

| Method            | Re-aggregates | Reasoning                                                                                                               |
| ----------------- | ------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `SetLocalMetrics` | yes           | a leaf's bounds change, so every ancestor may change                                                                    |
| `MoveNode`        | yes           | the subtree stops feeding the old ancestors and starts feeding the new ones; neither side is touched by the move itself |
| `DeleteNode`      | yes           | ancestors keep bounds contributed by now-deleted data                                                                   |
| `DeleteNodes`     | yes           | same, once for the whole batch                                                                                          |
| `AddNodes`        | yes           | the incoming nodes may carry non-zero local bounds                                                                      |
| `AddNode`         | **no**        | see below                                                                                                               |
| `InitializeTree`  | yes           | returns a tree that is already ordered and aggregated                                                                   |

`AddNode` is the single exception, and the reasoning is worth recording so it is not
"fixed" later. It creates a node with zero `LocalFirst` and `LocalLast`. The roll-up
skips zero-valued children, and no existing node changes parent, so every stored
aggregate is still correct. `AddNodes` cannot make the same argument, because its
signature permits populated nodes.

## Initialisation

`InitializeTree` takes a flat slice and performs **no I/O**. Loading is the
caller's concern, which is what makes the tree testable without a database.

```go
func InitializeTree(nodes []model.NodeTimeSpan) (*NodeTimeSpanService, error) {
	ts := &NodeTimeSpanService{
		nodes:    append([]model.NodeTimeSpan(nil), nodes...),
		indexMap: make(map[int32]int, len(nodes)),
	}

	// Both the aggregation sweep and the delete cascade depend on every parent
	// appearing before its children, so the ordering is established here rather
	// than assumed of the caller's slice.
	ts.rebuildFromInternalSlice()
	ts.recomputeAggregationsLocked()
	return ts, nil
}
```

Two things are established here rather than assumed:

- **Ordering is normalised.** The input may arrive in any order. The input slice is
  copied, so the caller's slice is never aliased.
- **Aggregates are computed.** The result is ready to serve reads immediately.

In production the slice comes from `timeSpanLoader` in `nodes.go`, which queries
the node table and the measurement store. That loader is shared by the initial
load and the decorator's periodic refresh, so the two cannot drift apart.

The tree service itself has no database or repository dependency at all.

## The aggregation sweep

```go
func (ts *NodeTimeSpanService) recomputeAggregationsLocked() {
	// Step A: reset all aggregates to the current local state
	for i := range ts.nodes {
		ts.nodes[i].AggFirst = ts.nodes[i].LocalFirst
		ts.nodes[i].AggLast = ts.nodes[i].LocalLast
	}

	// Step B: linear backward sweep (bottom-up). Since children are further
	// back, they have already been aggregated when the parent is reached.
	for i := len(ts.nodes) - 1; i > 0; i-- {
		pIdx := ts.nodes[i].ParentIdx
		if pIdx == -1 {
			continue // root or orphaned node
		}
		child := &ts.nodes[i]
		parent := &ts.nodes[pIdx]

		if !child.AggFirst.IsZero() {
			if parent.AggFirst.IsZero() || child.AggFirst.Before(parent.AggFirst) {
				parent.AggFirst = child.AggFirst
			}
		}
		if !child.AggLast.IsZero() {
			if parent.AggLast.IsZero() || child.AggLast.After(parent.AggLast) {
				parent.AggLast = child.AggLast
			}
		}
	}
}
```

Two linear passes, no allocation, no recursion. The caller must already hold the
write lock, which is why the name carries the `Locked` suffix — every mutator
already holds it.

The zero checks are what let partially-populated trees work. A folder with no
measurements of its own, or a series whose range could not be read, contributes
nothing rather than dragging a bound to the zero time.

## Structural changes

### Ordering is re-established, never patched

Every structural mutation calls `rebuildFromInternalSlice`, which re-sorts
breadth-first.

An earlier iteration appended a new node to the tail of the slice and then patched
its `ParentIdx` by looking the parent up in `indexMap`. That quietly **breaks the
parents-before-children invariant** whenever a node is added under a parent that
already has children: the new child lands after any of its own descendants, and the
backward sweep then aggregates the parent before the child, silently producing
wrong bounds. The ordering is now rebuilt rather than repaired.

```go
func (ts *NodeTimeSpanService) rebuildFromInternalSlice() {
	byParent := make(map[int32][]model.NodeTimeSpan)
	var rootNodes []model.NodeTimeSpan

	for _, n := range ts.nodes {
		if n.ParentID == 0 {
			rootNodes = append(rootNodes, n)
		} else {
			byParent[n.ParentID] = append(byParent[n.ParentID], n)
		}
	}

	ordered := make([]model.NodeTimeSpan, 0, len(ts.nodes))
	indexMap := make(map[int32]int)
	emitted := make(map[int32]bool, len(ts.nodes))

	queue := rootNodes
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if emitted[curr.ID] {
			continue
		}
		emitted[curr.ID] = true
		indexMap[curr.ID] = len(ordered)
		ordered = append(ordered, curr)

		if children, exists := byParent[curr.ID]; exists {
			queue = append(queue, children...)
		}
	}

	// A node whose parent is not part of this slice is never reached by the
	// walk. Keep it rather than dropping it; rebuildParentIndices gives it
	// ParentIdx = -1 so it still aggregates locally but no longer rolls up
	// into an ancestor.
	for _, n := range ts.nodes {
		if !emitted[n.ID] {
			emitted[n.ID] = true
			indexMap[n.ID] = len(ordered)
			ordered = append(ordered, n)
		}
	}

	ts.nodes = ordered
	ts.indexMap = indexMap
	ts.rebuildParentIndices()
}
```

Two properties are worth calling out:

- **The BFS walk is what establishes the ordering invariant.** It emits a parent
  before queueing its children, so the invariant holds by construction rather than
  by assumption about the input.
- **Orphans are preserved, not dropped.** A node whose parent is not in the loaded
  set — for example a kind the loader filtered out — is never reached by the walk.
  The second pass keeps it instead of losing it, and it aggregates locally while
  rolling up nowhere. Losing such a node would be a silent data-loss bug: a
  whole subtree vanishing from the tree because of one missing ancestor.

### Batch deletion

`DeleteNodes` takes a set of ids and removes each together with its descendants.

```go
func (ts *NodeTimeSpanService) DeleteNodes(nodeIDs []int32) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	deletedIDs := make(map[int32]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		// Unknown ids are dropped so that a batch naming only missing nodes
		// short-circuits instead of rebuilding an unchanged tree.
		if _, exists := ts.indexMap[id]; exists {
			deletedIDs[id] = true
		}
	}
	if len(deletedIDs) == 0 {
		return
	}

	// One forward pass suffices: rebuildFromInternalSlice keeps every parent
	// ahead of its children, so a node's parent is already in deletedIDs by
	// the time that node is reached, and the deletion cascades transitively.
	for i := range ts.nodes {
		if deletedIDs[ts.nodes[i].ParentID] {
			deletedIDs[ts.nodes[i].ID] = true
		}
	}

	newNodes := ts.nodes[:0]
	for _, n := range ts.nodes {
		if !deletedIDs[n.ID] {
			newNodes = append(newNodes, n)
		}
	}

	ts.nodes = newNodes
	ts.rebuildFromInternalSlice()
	ts.recomputeAggregationsLocked()
}
```

The node handler passes the **whole descendant set** to `DeleteNodesByIDs`, not
just the root of the removed subtree. Calling the singular delete once per id meant
one full-tree rebuild and one full aggregation sweep _per id_, which is quadratic in
subtree size. This is now a single pass, so deleting a folder with 500 descendants
costs the same as deleting one with 5.

The single forward cascade depends on the ordering invariant. If it were ever
violated, the failure mode is **under**-deletion — a child sitting before its
parent is missed, leaving dangling nodes — not over-deletion.

`DeleteNode` delegates to it:

```go
func (ts *NodeTimeSpanService) DeleteNode(nodeID int32) {
	ts.DeleteNodes([]int32{nodeID})
}
```

so the singular and batch paths cannot drift apart. The batch form is also
idempotent for overlapping input: naming both a folder and its child removes
exactly the same set as naming the folder alone.

## Read API

```go
func (ts *NodeTimeSpanService) GetTimeSpan(nodeID int32) (first, last time.Time, exists bool) {
	ts.mu.RLock() // RLock allows parallel read access
	defer ts.mu.RUnlock()

	idx, exists := ts.indexMap[nodeID]
	if !exists {
		return time.Time{}, time.Time{}, false
	}
	return ts.nodes[idx].AggFirst, ts.nodes[idx].AggLast, true
}
```

One map lookup and two field reads under a read lock. Because the service is
self-consistent, there is no need to guard against a read racing a re-aggregation
— the write lock excludes it.

`GetNodes` returns a copy, so a caller cannot corrupt the tree by mutating the
result.

## Testing

The service is tested directly, with no database and no decorator, since it has no
I/O. Twelve test functions with 50 subtests in total cover:

- aggregation, including zero times and partially-populated nodes
- the ordering invariant, asserted with a `requireParentBeforeChild` helper after
  every structural change
- `GetTimeSpan` after **each** mutator, which is the regression guard for the
  consistency invariant described above
- the delete cascade: disjoint roots, overlapping input, unknown ids, empty input,
  grandchild transitivity, orphan preservation
- concurrent reads and writes

The `GetTimeSpan`-after-mutation tests are the ones that matter. The original suite
asserted only structure after mutations — slice length, `indexMap`, `ParentIdx` —
and never read a time span, which is why the stale-aggregate bug survived. Removing
the re-aggregation from any mutator now fails at least one of them.

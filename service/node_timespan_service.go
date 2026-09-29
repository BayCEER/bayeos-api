package service

import (
	"sync"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

// NodeTimeSpanServiceInterface exposes a tree of per-node time spans. Every
// method leaves the tree internally consistent: aggregates are always correct
// for the current topology, so callers never have to repair them afterwards.
// Adding a public "recompute" step would make that a caller obligation, and it
// was routinely forgotten.
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

type NodeTimeSpanService struct {
	mu       sync.RWMutex
	nodes    []model.NodeTimeSpan
	indexMap map[int32]int
}

func NewNodeTimeSpanService() *NodeTimeSpanService {
	return &NodeTimeSpanService{
		nodes:    []model.NodeTimeSpan{},
		indexMap: make(map[int32]int),
	}
}

// InitializeTree builds a span service from a flat node list. The input order
// does not matter: the tree is re-sorted parents-before-children and
// aggregated before it is returned, so the result is ready to serve reads.
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

// rebuildParentIndices converts int32 relationships into direct array indices
func (ts *NodeTimeSpanService) rebuildParentIndices() {
	for i := range ts.nodes {
		if ts.nodes[i].ParentID == 0 {
			ts.nodes[i].ParentIdx = -1
			continue
		}
		if idx, exists := ts.indexMap[ts.nodes[i].ParentID]; exists {
			ts.nodes[i].ParentIdx = int32(idx)
		} else {
			ts.nodes[i].ParentIdx = -1 // Catches orphaned nodes
		}
	}
}

// recomputeAggregationsLocked calculates min(first) and max(last) from bottom to
// top. The caller must already hold ts.mu for writing.
func (ts *NodeTimeSpanService) recomputeAggregationsLocked() {
	// Step A: Reset all aggregates to the current local state
	for i := range ts.nodes {
		ts.nodes[i].AggFirst = ts.nodes[i].LocalFirst
		ts.nodes[i].AggLast = ts.nodes[i].LocalLast
	}

	// Step B: Linear backward sweep (Bottom-Up)
	// Since children are further back, they have already been aggregated when the parent is reached.
	for i := len(ts.nodes) - 1; i > 0; i-- {
		pIdx := ts.nodes[i].ParentIdx
		if pIdx == -1 {
			continue // Root or orphaned node
		}

		child := &ts.nodes[i]
		parent := &ts.nodes[pIdx]

		// Roll up: min(First)
		if !child.AggFirst.IsZero() {
			if parent.AggFirst.IsZero() || child.AggFirst.Before(parent.AggFirst) {
				parent.AggFirst = child.AggFirst
			}
		}

		// Roll up: max(Last)
		if !child.AggLast.IsZero() {
			if parent.AggLast.IsZero() || child.AggLast.After(parent.AggLast) {
				parent.AggLast = child.AggLast
			}
		}
	}
}

// SetLocalMetrics records the local bounds of a node and re-aggregates its
// ancestors. The full sweep is a few microseconds for 50K nodes, which is
// cheaper than maintaining per-node deltas up a chain.
func (ts *NodeTimeSpanService) SetLocalMetrics(nodeID int32, first, last time.Time) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	idx, exists := ts.indexMap[nodeID]
	if !exists {
		return
	}
	ts.nodes[idx].LocalFirst = first
	ts.nodes[idx].LocalLast = last
	ts.recomputeAggregationsLocked()
}

// AddNode adds a new node to the Span
func (ts *NodeTimeSpanService) AddNode(nodeID int32, parentID int32) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Check if the node already exists
	if _, exists := ts.indexMap[nodeID]; exists {
		return // Node already exists
	}

	// Create new node
	newNode := model.NodeTimeSpan{
		ID:        nodeID,
		ParentID:  parentID,
		ParentIdx: -1,
	}

	// Add to the internal list
	ts.nodes = append(ts.nodes, newNode)

	// The internal slice must stay ordered parents-before-children, because
	// the aggregation sweep runs backwards over it. A tail append places the
	// new node after any of its own descendants, so the order is rebuilt.
	ts.rebuildFromInternalSlice()

	// No re-aggregation is needed: the new node carries zero Local bounds, the
	// roll-up skips zero children, and no existing node changed parent. Every
	// stored aggregate is therefore still correct.
}

// AddNodes adds multiple new nodes to the Span
func (ts *NodeTimeSpanService) AddNodes(nodes []model.NodeTimeSpan) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Check which nodes already exist
	existingNodes := make(map[int32]bool)
	for _, node := range nodes {
		if _, exists := ts.indexMap[node.ID]; exists {
			existingNodes[node.ID] = true
		}
	}

	// Add only new nodes
	added := false
	for _, node := range nodes {
		if !existingNodes[node.ID] {
			ts.nodes = append(ts.nodes, node)
			added = true
		}
	}
	if !added {
		return
	}

	// Restore the parents-before-children ordering the backward aggregation
	// sweep depends on; see AddNode.
	ts.rebuildFromInternalSlice()

	// Unlike AddNode, the incoming nodes may carry non-zero Local bounds, so
	// the ancestors of each added node have to pick them up.
	ts.recomputeAggregationsLocked()
}

// MoveNode moves a node to a new parent and rebuilds the topology
func (ts *NodeTimeSpanService) MoveNode(nodeID int32, newParentID int32) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	idx, exists := ts.indexMap[nodeID]
	if !exists {
		return
	}

	// Update structure
	ts.nodes[idx].ParentID = newParentID

	// Since the topology has changed, use the BFS algorithm to re-sort
	ts.rebuildFromInternalSlice()

	// The subtree stops feeding the old ancestors and starts feeding the new
	// ones, and neither side is updated by the move itself.
	ts.recomputeAggregationsLocked()
}

// DeleteNode removes a node and all its sub-children (cascading).
func (ts *NodeTimeSpanService) DeleteNode(nodeID int32) {
	ts.DeleteNodes([]int32{nodeID})
}

// DeleteNodes removes every given node together with all of its descendants, and
// re-aggregates once for the whole batch. Deleting a subtree that spans many
// nodes costs the same as deleting a single one, instead of a full rebuild per
// id.
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
	// ahead of its children, so a node's parent is already in deletedIDs by the
	// time that node is reached, and the deletion cascades transitively.
	for i := range ts.nodes {
		if deletedIDs[ts.nodes[i].ParentID] {
			deletedIDs[ts.nodes[i].ID] = true
		}
	}

	// In-place filtering of the slice
	newNodes := ts.nodes[:0]
	for _, n := range ts.nodes {
		if !deletedIDs[n.ID] {
			newNodes = append(newNodes, n)
		}
	}

	ts.nodes = newNodes
	ts.rebuildFromInternalSlice()

	// Removed data contributed to the bounds of every ancestor, and a stale
	// bound can only err outwards (an old first, a late last), so it never
	// corrects itself.
	ts.recomputeAggregationsLocked()
}

// Helper function for internal rebuild after structural changes
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

	// A node whose parent is not part of this slice (e.g. a kind the loader
	// filtered out) is never reached by the walk. Keep it rather than dropping
	// it; rebuildParentIndices gives it ParentIdx = -1 so it still aggregates
	// locally but no longer rolls up into an ancestor.
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

func (ts *NodeTimeSpanService) GetTimeSpan(nodeID int32) (first time.Time, last time.Time, exists bool) {
	ts.mu.RLock() // RLock allows parallel read access
	defer ts.mu.RUnlock()

	idx, exists := ts.indexMap[nodeID]
	if !exists {
		return time.Time{}, time.Time{}, false
	}

	return ts.nodes[idx].AggFirst, ts.nodes[idx].AggLast, true
}

// GetNodes returns a copy of the nodes for testing purposes
func (ts *NodeTimeSpanService) GetNodes() []model.NodeTimeSpan {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	nodesCopy := make([]model.NodeTimeSpan, len(ts.nodes))
	copy(nodesCopy, ts.nodes)
	return nodesCopy
}

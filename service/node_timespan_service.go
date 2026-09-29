package service

import (
	"sync"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

type NodeTimeSpanServiceInterface interface {
	RecomputeAggregations()
	SetLocalMetrics(nodeID int32, first, last time.Time)
	AddNode(nodeID int32, parentID int32)
	AddNodes(nodes []model.NodeTimeSpan)
	MoveNode(nodeID int32, newParentID int32)
	DeleteNode(nodeID int32)
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

// InitializeTreeFromNodes initializes the Span from an array of Node structs
// Assumes the input nodes array is already sorted in the correct order
// (parents before children)
func InitializeTree(nodes []model.NodeTimeSpan) (*NodeTimeSpanService, error) {
	indexMap := make(map[int32]int)

	// Build indexMap directly from the input array
	for i, node := range nodes {
		indexMap[node.ID] = i
	}

	ts := &NodeTimeSpanService{
		nodes:    nodes,
		indexMap: indexMap,
	}

	// Build index cache
	ts.rebuildParentIndices()
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

// RecomputeAggregations calculates min(first) and max(last) from bottom to top.
// Due to the flat memory layout, this runs in a few microseconds for 50K nodes.
func (ts *NodeTimeSpanService) RecomputeAggregations() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

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

// SetLocalMetrics allows fast setting of data before a Recompute call
func (ts *NodeTimeSpanService) SetLocalMetrics(nodeID int32, first, last time.Time) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if idx, exists := ts.indexMap[nodeID]; exists {
		ts.nodes[idx].LocalFirst = first
		ts.nodes[idx].LocalLast = last
	}
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

	// Set index for the new node
	ts.indexMap[nodeID] = len(ts.nodes)

	// Add to the internal list
	ts.nodes = append(ts.nodes, newNode)

	// Set ParentIdx for the new node
	if parentID != 0 {
		if parentIdx, exists := ts.indexMap[parentID]; exists {
			ts.nodes[len(ts.nodes)-1].ParentIdx = int32(parentIdx)
		}
	}
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
	for _, node := range nodes {
		if !existingNodes[node.ID] {
			// Set index for the new node
			ts.indexMap[node.ID] = len(ts.nodes)

			// Add to the internal list
			ts.nodes = append(ts.nodes, node)

			// Set ParentIdx for the new node
			if node.ParentID != 0 {
				if parentIdx, exists := ts.indexMap[node.ParentID]; exists {
					ts.nodes[len(ts.nodes)-1].ParentIdx = int32(parentIdx)
				}
			}
		}
	}
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
}

// DeleteNode removes a node and all its sub-children (cascading)
func (ts *NodeTimeSpanService) DeleteNode(nodeID int32) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	deletedIDs := make(map[int32]bool)
	deletedIDs[nodeID] = true

	// Use the existing sorting: children are always further back!
	for i := 0; i < len(ts.nodes); i++ {
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

	queue := rootNodes
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		indexMap[curr.ID] = len(ordered)
		ordered = append(ordered, curr)

		if children, exists := byParent[curr.ID]; exists {
			queue = append(queue, children...)
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

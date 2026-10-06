package service

import (
	"sync"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

// PropertyRule declares that the metadata of a node of SourceKind is inherited
// by that node's descendants. A rule says nothing about the kind of the
// receiving node: a Folder carrying unit passes it to every descendant
// regardless of whether that descendant is a Folder or a Timeseries.
//
// A rule is scoped to a kind, not to a property. Every key a source node holds
// is inherited, so a property nobody anticipated still reaches its descendants
// and the set of inheritable properties cannot be narrowed from here.
type PropertyRule struct {
	// SourceKind is the node kind whose values are inherited. Empty matches any
	// kind, which is rarely what is wanted, because it lets a Timeseries
	// override a value intended to come from the folder above it.
	SourceKind string
}

// NodePropertyServiceInterface resolves inherited metadata properties from an
// in-memory tree of local values.
//
// The tree stores only what is actually stored on each node. Inherited values
// are never materialised, so a change to an ancestor takes effect on the next
// read with no rebuild, and a value can never drift into being local.
type NodePropertyServiceInterface interface {
	// Inherited returns the properties nodeID would inherit from its
	// ancestors, keyed by property name. It returns nil when the node is
	// unknown or has nothing to inherit.
	//
	// Nearest ancestor wins: the walk starts at the parent, so a value set on
	// an intermediate folder overrides the same value further up.
	Inherited(nodeID int32) map[string]any

	SetLocalMetadata(nodeID int32, metadata model.Metadata)
	AddNode(nodeID int32, parentID int32, kind string, metadata model.Metadata)
	DeleteNodes(nodeIDs []int32)
	MoveNode(nodeID int32, newParentID int32)
	GetNodes() []model.Node
}

// NodePropertyService is the in-memory implementation of
// NodePropertyServiceInterface. A node is addressed by id rather than by slice
// index, because inheritance walks up by parent id, so there is no ordering
// invariant to maintain and no need for a rebuild after a structural change.
type NodePropertyService struct {
	mu    sync.RWMutex
	nodes map[int32]model.Node
	rules []PropertyRule
}

// NewNodePropertyService returns an empty property service.
func NewNodePropertyService() *NodePropertyService {
	return &NodePropertyService{
		nodes: make(map[int32]model.Node),
		rules: []PropertyRule{},
	}
}

// InitializePropertyTree builds a property service from a flat node list.
//
// The input order does not matter: lookups follow parent ids rather than slice
// positions, so the result is ready to serve reads as soon as it is returned.
func InitializePropertyTree(nodes []model.Node, rules []PropertyRule) (*NodePropertyService, error) {
	ps := NewNodePropertyService()
	ps.rules = append(ps.rules, rules...)
	for _, n := range nodes {
		ps.nodes[n.ID] = localNode(n)
	}
	return ps, nil
}

// localNode strips the inherited marker from a loaded node. The loader reads the
// stored metadata, which must never carry the marker; dropping it here means a
// row that somehow does carry it is treated as holding purely local values
// instead of being able to feed its own inherited keys back to descendants.
func localNode(n model.Node) model.Node {
	out := n
	out.Metadata = model.StripComputed(n.Metadata)
	return out
}

// Inherited walks the ancestor chain and collects the nearest value of every
// metadata key held by a source node.
//
// The walk starts at the parent, so a node that both declares a key locally and
// has an ancestor declaring it still reports the key: it IS inherited, even
// though the local value is what ends up in the effective metadata.
//
// The walk has to reach the root. A key that only an upper ancestor holds is
// inherited just the same, and a nearer ancestor that does not hold it cannot be
// told apart from one that was never going to, so each source node contributes
// what it is still missing instead of replacing what was already collected.
func (ps *NodePropertyService) Inherited(nodeID int32) map[string]any {
	if len(ps.rules) == 0 {
		return nil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if _, ok := ps.nodes[nodeID]; !ok {
		return nil
	}

	out := map[string]any{}
	seen := map[int32]bool{}

	cur := ps.nodes[nodeID].ParentID
	for cur != 0 {
		// A malformed tree can contain a parent cycle. Without this guard the
		// walk would spin forever on data the loader accepted.
		if seen[cur] {
			break
		}
		seen[cur] = true

		node, ok := ps.nodes[cur]
		if !ok {
			break
		}
		if ps.supplies(node.Kind) {
			// The tree holds only locally stored values, so every key here is a
			// real property of the node: a computed one was stripped on the way
			// in and cannot be inherited back down.
			for key, value := range node.Metadata.Map() {
				if _, done := out[key]; done {
					continue
				}
				out[key] = value
			}
		}
		cur = node.ParentID
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// supplies reports whether a node of the given kind contributes its metadata to
// its descendants.
func (ps *NodePropertyService) supplies(kind string) bool {
	for _, rule := range ps.rules {
		if rule.SourceKind == "" || rule.SourceKind == kind {
			return true
		}
	}
	return false
}

// SetLocalMetadata records the local metadata of a node. An unknown node is
// ignored, matching the time span service, rather than creating a node whose
// ancestry is unknown.
//
// The patch is merged, not substituted, because that is what the column does.
// UpdateNodeMetadata merges the patch into the document, so a client that
// sends one key adds one key, and a key sent as null removes that key.
// Replacing the mirrored document here would drop every other local key from
// the mirror, and every later read would then resolve those keys as if the node
// had never had them.
func (ps *NodePropertyService) SetLocalMetadata(nodeID int32, metadata model.Metadata) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	node, ok := ps.nodes[nodeID]
	if !ok {
		return
	}
	node.Metadata = model.MergeMetadata(node.Metadata, model.StripComputed(metadata))
	ps.nodes[nodeID] = node
}

// AddNode registers a new node. The stored metadata is reduced to its local
// keys, so a caller that echoes a decorated node back cannot seed the tree with
// inherited values as if they were local.
func (ps *NodePropertyService) AddNode(nodeID int32, parentID int32, kind string, metadata model.Metadata) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, exists := ps.nodes[nodeID]; exists {
		return
	}
	ps.nodes[nodeID] = model.Node{
		ID:       nodeID,
		ParentID: parentID,
		Kind:     kind,
		Metadata: model.StripComputed(metadata),
	}
}

// DeleteNodes removes the given nodes together with all of their descendants.
func (ps *NodePropertyService) DeleteNodes(nodeIDs []int32) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	deleted := make(map[int32]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		if _, ok := ps.nodes[id]; ok {
			deleted[id] = true
		}
	}
	if len(deleted) == 0 {
		return
	}

	// The set grows as the sweep runs, so a deletion of a subtree cascades
	// transitively. Go randomises map iteration, so a node visited before its
	// parent was marked would otherwise survive; the loop repeats until the set
	// stops growing, which makes the result independent of iteration order.
	for len(deleted) < len(ps.nodes) {
		grown := len(deleted)
		for _, n := range ps.nodes {
			if deleted[n.ParentID] {
				deleted[n.ID] = true
			}
		}
		if len(deleted) == grown {
			break
		}
	}

	for id := range deleted {
		delete(ps.nodes, id)
	}
}

// MoveNode re-parents a node, which changes the ancestors it inherits from.
func (ps *NodePropertyService) MoveNode(nodeID int32, newParentID int32) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	node, ok := ps.nodes[nodeID]
	if !ok {
		return
	}
	node.ParentID = newParentID
	ps.nodes[nodeID] = node
}

// GetNodes returns a copy of the tracked nodes, for diagnostics and refresh
// bookkeeping. The slice is safe to modify.
func (ps *NodePropertyService) GetNodes() []model.Node {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	out := make([]model.Node, 0, len(ps.nodes))
	for _, n := range ps.nodes {
		out = append(out, n)
	}
	return out
}

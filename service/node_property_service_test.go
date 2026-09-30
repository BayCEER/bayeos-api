package service

import (
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// folderRule is the rule most cases share: everything a Folder holds flows down.
var folderRule = PropertyRule{SourceKind: model.KindFolder}

// propNode builds a node of the given kind with the given metadata.
func propNode(id, parent int32, kind string, md map[string]any) model.Node {
	metadata, err := model.MetadataFromMap(md)
	if err != nil {
		panic(err)
	}
	return model.Node{ID: id, ParentID: parent, Kind: kind, Metadata: metadata}
}

// newProps builds a tree from nodes and returns it.
func newProps(t *testing.T, nodes []model.Node, rules ...PropertyRule) *NodePropertyService {
	t.Helper()
	ps, err := InitializePropertyTree(nodes, rules)
	require.NoError(t, err)
	return ps
}

func TestInheritedNearestAncestorWins(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		nodes []model.Node
		want  any
	}{
		"root value reaches the leaf": {
			nodes: []model.Node{
				propNode(1, 0, model.KindFolder, map[string]any{"unit": "°C"}),
				propNode(2, 1, model.KindFolder, nil),
				propNode(3, 2, model.KindTimeseries, nil),
			},
			want: "°C",
		},
		"intermediate folder overrides the root": {
			nodes: []model.Node{
				propNode(1, 0, model.KindFolder, map[string]any{"unit": "K"}),
				propNode(2, 1, model.KindFolder, map[string]any{"unit": "°C"}),
				propNode(3, 2, model.KindTimeseries, nil),
			},
			want: "°C",
		},
		"nearest ancestor without the key does not shadow": {
			nodes: []model.Node{
				propNode(1, 0, model.KindFolder, map[string]any{"unit": "K"}),
				propNode(2, 1, model.KindFolder, map[string]any{"other": "x"}),
				propNode(3, 2, model.KindTimeseries, nil),
			},
			want: "K",
		},
		"a null value still counts as set": {
			nodes: []model.Node{
				propNode(1, 0, model.KindFolder, map[string]any{"unit": nil}),
				propNode(2, 1, model.KindFolder, nil),
				propNode(3, 2, model.KindTimeseries, nil),
			},
			want: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ps := newProps(t, tc.nodes, folderRule)

			got := ps.Inherited(3)
			require.NotNil(t, got)
			value, present := got["unit"]
			assert.True(t, present, "unit should be reported as inherited")
			if tc.want != nil {
				assert.Equal(t, tc.want, value)
			} else {
				assert.Nil(t, value)
			}
		})
	}
}

func TestInheritedSourceKindIsHonoured(t *testing.T) {
	t.Parallel()

	// A Timeseries value must not satisfy a rule that names Folder as the
	// source kind, even though it sits on an ancestor.
	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, nil),
		propNode(2, 1, model.KindTimeseries, map[string]any{"unit": "V"}),
		propNode(3, 2, model.KindTimeseries, nil),
	}, folderRule)

	assert.Nil(t, ps.Inherited(3))
}

func TestInheritedSkipsNonMatchingIntermediateNodes(t *testing.T) {
	t.Parallel()

	// The series on node 2 does not match the rule's source kind, so it
	// contributes nothing: neither its unit nor a key only it holds.
	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "A"}),
		propNode(2, 1, model.KindTimeseries, map[string]any{"unit": "B", "own": "series"}),
		propNode(3, 2, model.KindTimeseries, nil),
	}, folderRule)

	got := ps.Inherited(3)
	assert.Equal(t, "A", got["unit"])
	assert.NotContains(t, got, "own", "a non-source kind supplies nothing")
}

// A rule names a kind, not a property, so every key a source node holds is
// inherited whether or not anything configured it by name.
func TestInheritedTakesEveryKeyOfTheSource(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{
			"unit": "°C", "resolution": 60.0, "location": "hall",
		}),
		propNode(2, 1, model.KindFolder, map[string]any{"resolution": 30.0}),
		propNode(3, 2, model.KindTimeseries, nil),
	}, folderRule)

	assert.Equal(t, map[string]any{
		"unit":       "°C",
		"resolution": 30.0,
		"location":   "hall",
	}, ps.Inherited(3),
		"the nearer folder wins for the key it shares and the root supplies the rest")
}

func TestInheritedRootHasNoAncestors(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "°C"}),
	}, folderRule)

	// A node cannot inherit from itself, even though it holds the value.
	assert.Nil(t, ps.Inherited(1))
}

func TestInheritedUnknownAndOrphanNodes(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "°C"}),
		propNode(2, 1, model.KindTimeseries, nil),
		// Parent 99 does not exist, so the walk stops immediately.
		propNode(3, 99, model.KindTimeseries, nil),
	}, folderRule)

	assert.Nil(t, ps.Inherited(404))
	assert.Nil(t, ps.Inherited(3))
}

func TestInheritedWithoutRules(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "°C"}),
		propNode(2, 1, model.KindTimeseries, nil),
	})

	assert.Nil(t, ps.Inherited(2))
}

func TestInheritedTerminatesOnParentCycle(t *testing.T) {
	t.Parallel()

	// A malformed tree must not hang the request thread.
	ps := newProps(t, []model.Node{
		propNode(1, 2, model.KindFolder, nil),
		propNode(2, 1, model.KindFolder, map[string]any{"unit": "°C"}),
		propNode(3, 1, model.KindTimeseries, nil),
	}, folderRule)

	assert.NotPanics(t, func() { ps.Inherited(3) })
	assert.Equal(t, "°C", ps.Inherited(3)["unit"])
}

func TestSetLocalMetadataUpdatesInheritance(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "K"}),
		propNode(2, 1, model.KindTimeseries, nil),
	}, folderRule)
	assert.Equal(t, "K", ps.Inherited(2)["unit"])

	md, err := model.MetadataFromMap(map[string]any{"unit": "°C"})
	require.NoError(t, err)
	ps.SetLocalMetadata(1, md)

	assert.Equal(t, "°C", ps.Inherited(2)["unit"])
}

// A metadata write is additive in SQL, so the mirror has to be too. Replacing
// it would make a node look as though it had never had the other local keys, and
// every later read would then resolve them from an ancestor instead.
func TestSetLocalMetadataMergesRatherThanReplaces(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "K", "own": "x"}),
		propNode(2, 1, model.KindTimeseries, map[string]any{"own": "child"}),
	}, folderRule)

	// A patch that names one key, exactly as UpdateNodeMetadata would send it.
	patch, err := model.MetadataFromMap(map[string]any{"unit": "°C"})
	require.NoError(t, err)
	ps.SetLocalMetadata(1, patch)

	assert.Equal(t, "°C", ps.Inherited(2)["unit"], "the patched key takes effect")
	assert.Equal(t, "x", localMetadataOf(t, ps, 1)["own"],
		"a key the patch did not name must survive, or the mirror drifts from the column")
}

// The mirror is reduced to local values on the way in, so an echoed node cannot
// seed the tree with derived values as if they were local.
func TestSetLocalMetadataDropsComputedKeys(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, nil),
		propNode(2, 1, model.KindTimeseries, nil),
	}, folderRule)

	echo, err := model.MetadataFromMap(map[string]any{
		"own":               "x",
		"unit":              "°C",
		"first":             "2020-01-01T00:00:00Z",
		model.InheritedKey:  []any{"unit"},
		model.AggregatedKey: []any{"first"},
	})
	require.NoError(t, err)
	ps.SetLocalMetadata(2, echo)

	stored := localMetadataOf(t, ps, 2)
	assert.Equal(t, "x", stored["own"])
	assert.NotContains(t, stored, "unit", "an inherited value must not become local")
	assert.NotContains(t, stored, "first", "a span value must not become local")
	assert.NotContains(t, stored, model.InheritedKey)
	assert.NotContains(t, stored, model.AggregatedKey)
}

// localMetadataOf returns the stored local metadata of one node. The service
// exposes no accessor for it, so it is read back through the public node list.
func localMetadataOf(t *testing.T, ps *NodePropertyService, id int32) map[string]any {
	t.Helper()
	for _, n := range ps.GetNodes() {
		if n.ID == id {
			return n.Metadata.Map()
		}
	}
	t.Fatalf("node %d is not in the property tree", id)
	return nil
}

func TestSetLocalMetadataOnUnknownNodeIsIgnored(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{propNode(1, 0, model.KindFolder, nil)}, folderRule)
	md, err := model.MetadataFromMap(map[string]any{"unit": "°C"})
	require.NoError(t, err)

	assert.NotPanics(t, func() { ps.SetLocalMetadata(42, md) })
	assert.Len(t, ps.GetNodes(), 1)
}

func TestAddNodeRegistersInheritance(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "°C"}),
	}, folderRule)

	ps.AddNode(2, 1, model.KindTimeseries, model.NewMetadata(nil))

	assert.Equal(t, "°C", ps.Inherited(2)["unit"])
}

func TestAddNodeIsIdempotent(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "K"}),
	}, folderRule)

	// Re-adding an existing node must not overwrite the stored local value.
	md, err := model.MetadataFromMap(map[string]any{"unit": "°C"})
	require.NoError(t, err)
	ps.AddNode(1, 0, model.KindFolder, md)

	assert.Len(t, ps.GetNodes(), 1)
	assert.Equal(t, "K", ps.GetNodes()[0].Metadata.Map()["unit"])
}

func TestAddNodeDropsInheritedKeysFromStoredMetadata(t *testing.T) {
	t.Parallel()

	// A caller echoing a decorated node must not seed the tree with derived
	// values as if they were local.
	ps := newProps(t, []model.Node{propNode(1, 0, model.KindFolder, nil)}, folderRule)

	decorated, err := model.MetadataFromMap(map[string]any{
		"unit":             "°C",
		"local":            "mine",
		model.InheritedKey: []any{"unit"},
	})
	require.NoError(t, err)
	ps.AddNode(2, 1, model.KindFolder, decorated)

	for _, n := range ps.GetNodes() {
		if n.ID != 2 {
			continue
		}
		md := n.Metadata.Map()
		assert.NotContains(t, md, model.InheritedKey)
		assert.NotContains(t, md, "unit", "an inherited key must not be stored as local")
		assert.Equal(t, "mine", md["local"])
	}
}

func TestMoveNodeChangesInheritedValue(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "A"}),
		propNode(2, 0, model.KindFolder, map[string]any{"unit": "B"}),
		propNode(3, 1, model.KindTimeseries, nil),
	}, folderRule)
	assert.Equal(t, "A", ps.Inherited(3)["unit"])

	ps.MoveNode(3, 2)
	assert.Equal(t, "B", ps.Inherited(3)["unit"])
}

func TestMoveUnknownNodeIsIgnored(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{propNode(1, 0, model.KindFolder, nil)}, folderRule)
	assert.NotPanics(t, func() { ps.MoveNode(404, 1) })
}

func TestDeleteNodesCascades(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "°C"}),
		propNode(2, 1, model.KindFolder, nil),
		propNode(3, 2, model.KindTimeseries, nil),
		propNode(4, 3, model.KindTimeseries, nil),
		// An unrelated subtree that must be untouched.
		propNode(5, 0, model.KindFolder, map[string]any{"unit": "V"}),
		propNode(6, 5, model.KindTimeseries, nil),
	}, folderRule)

	ps.DeleteNodes([]int32{2})

	// The whole subtree is gone, the unrelated node survives, and the deleted
	// ids no longer resolve.
	assert.Len(t, ps.GetNodes(), 3)
	assert.Nil(t, ps.Inherited(2))
	assert.Nil(t, ps.Inherited(3))
	assert.Nil(t, ps.Inherited(4))
	assert.Equal(t, "V", ps.Inherited(6)["unit"])
}

func TestDeleteNodesCascadesThroughDeepChain(t *testing.T) {
	t.Parallel()

	// A chain deeper than the cascade can follow in one pass. Go randomises map
	// iteration, so a single sweep would leave nodes behind depending on the
	// order the map happened to yield.
	var nodes []model.Node
	const depth = 25
	for i := int32(1); i <= depth; i++ {
		parent := int32(0)
		if i > 1 {
			parent = i - 1
		}
		nodes = append(nodes, propNode(i, parent, model.KindFolder, nil))
	}
	ps := newProps(t, nodes, folderRule)

	ps.DeleteNodes([]int32{1})

	assert.Empty(t, ps.GetNodes(), "every descendant of a deleted root goes with it")
}

func TestDeleteNodesUnknownAndEmpty(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{"unit": "°C"}),
		propNode(2, 1, model.KindTimeseries, nil),
	}, folderRule)

	before := len(ps.GetNodes())
	assert.NotPanics(t, func() { ps.DeleteNodes(nil) })
	assert.NotPanics(t, func() { ps.DeleteNodes([]int32{}) })
	assert.NotPanics(t, func() { ps.DeleteNodes([]int32{404, 405}) })
	assert.Len(t, ps.GetNodes(), before)
}

func TestInitializeIgnoresStoredInheritedMarker(t *testing.T) {
	t.Parallel()

	// A row that somehow carries the marker is treated as holding only local
	// values, so it cannot feed its own inherited keys to its descendants.
	ps := newProps(t, []model.Node{
		propNode(1, 0, model.KindFolder, map[string]any{
			"unit":             "°C",
			"local":            "mine",
			model.InheritedKey: []any{"unit"},
		}),
		propNode(2, 1, model.KindTimeseries, nil),
	}, folderRule)

	got := ps.Inherited(2)
	require.NotNil(t, got)
	assert.Equal(t, "mine", got["local"], "a genuinely local key is still inherited")
	assert.NotContains(t, got, "unit", "the marker must not be trusted as a local source")
}

func TestGetNodesReturnsACopy(t *testing.T) {
	t.Parallel()

	ps := newProps(t, []model.Node{propNode(1, 0, model.KindFolder, nil)}, folderRule)

	nodes := ps.GetNodes()
	nodes[0].Kind = "mutated"

	assert.Equal(t, model.KindFolder, ps.GetNodes()[0].Kind)
}

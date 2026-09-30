package decorator

import (
	"context"
	"testing"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProps is a NodePropertyServiceInterface that returns a canned answer and
// records the calls it receives, so the decorator tests do not need a real tree.
type stubProps struct {
	service.NodePropertyServiceInterface

	inherited map[string]any
	deleted   []int32
	moved     map[int32]int32
	local     map[int32]model.Metadata
}

func newStubProps() *stubProps {
	return &stubProps{
		moved: map[int32]int32{},
		local: map[int32]model.Metadata{},
	}
}

func (s *stubProps) Inherited(_ int32) map[string]any { return s.inherited }
func (s *stubProps) SetLocalMetadata(id int32, md model.Metadata) {
	s.local[id] = md
}
func (s *stubProps) AddNode(int32, int32, string, model.Metadata) {}
func (s *stubProps) DeleteNodes(ids []int32)                      { s.deleted = ids }
func (s *stubProps) MoveNode(id, parent int32)                    { s.moved[id] = parent }
func (s *stubProps) GetNodes() []model.Node                       { return nil }

// inheritedChain builds a base service plus an InheritedProps decorator over a
// stub property service.
func inheritedChain(props service.NodePropertyServiceInterface, base service.NodeService) service.NodeService {
	return Chain(base, InheritedProps(InheritedConfig{
		Props:      props,
		Rules:      []service.PropertyRule{{SourceKind: model.KindFolder}},
		SyncWrites: true,
	}))
}

func TestInheritedPropsMergesAndMarks(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	props.inherited = map[string]any{"unit": "°C"}

	base := &stubNode{node: newNode(2, model.KindTimeseries, map[string]any{"lat": 1.0})}
	svc := inheritedChain(props, base)

	n, err := svc.GetNode(context.Background(), 2, 7)
	require.NoError(t, err)

	md := mdOf(n)
	assert.Equal(t, "°C", md["unit"], "inherited value is merged in")
	assert.Equal(t, 1.0, md["lat"], "existing metadata is preserved")
	assert.Equal(t, []any{"unit"}, md[model.InheritedKey], "the injected keys are marked")
}

func TestInheritedPropsDoesNotOverwriteLocalValue(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	props.inherited = map[string]any{"unit": "°C"}

	base := &stubNode{node: newNode(2, model.KindTimeseries, map[string]any{"unit": "V"})}
	svc := inheritedChain(props, base)

	n, err := svc.GetNode(context.Background(), 2, 7)
	require.NoError(t, err)

	md := mdOf(n)
	assert.Equal(t, "V", md["unit"], "a local value is never overwritten")
	// The key is absent from the marker. The ancestor did have a value for it,
	// but the value the client sees is the node's own, so the node is free to
	// change it. Listing it would refuse a write the node is entitled to make.
	assert.NotContains(t, md, model.InheritedKey)
}

// A key the node shadows and a key it does not are reported differently, so the
// marker still names what was actually injected.
func TestInheritedPropsMarksOnlyTheKeysItInjected(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	props.inherited = map[string]any{"unit": "°C", "lat": 1.0}

	base := &stubNode{node: newNode(2, model.KindTimeseries, map[string]any{"unit": "V"})}
	svc := inheritedChain(props, base)

	n, err := svc.GetNode(context.Background(), 2, 7)
	require.NoError(t, err)

	md := mdOf(n)
	assert.Equal(t, "V", md["unit"], "the local value wins")
	assert.Equal(t, 1.0, md["lat"], "the inherited value is filled in")
	assert.Equal(t, []any{"lat"}, md[model.InheritedKey])
}

func TestInheritedPropsNoMarkerWhenNothingInherited(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{node: newNode(2, model.KindTimeseries, map[string]any{"lat": 1.0})}
	svc := inheritedChain(props, base)

	n, err := svc.GetNode(context.Background(), 2, 7)
	require.NoError(t, err)

	md := mdOf(n)
	assert.NotContains(t, md, model.InheritedKey, "an untouched node stays unchanged")
	assert.Equal(t, map[string]any{"lat": 1.0}, md)
}

func TestInheritedPropsMarkerIsSorted(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	props.inherited = map[string]any{"unit": "°C", "resolution": 60.0, "lat": 1.0}
	svc := inheritedChain(props, &stubNode{node: newNode(2, model.KindTimeseries, nil)})

	n, err := svc.GetNode(context.Background(), 2, 7)
	require.NoError(t, err)

	// Map iteration order is random, so a stable marker is what makes the
	// response comparable.
	assert.Equal(t, []any{"lat", "resolution", "unit"}, mdOf(n)[model.InheritedKey])
}

func TestInheritedPropsAppliesToEveryNodeReturningMethod(t *testing.T) {
	t.Parallel()

	// Each case returns the nodes from the method under test, so a method that
	// stopped being decorated cannot pass by way of some other method's result.
	tests := map[string]func(service.NodeService) ([]model.Node, error){
		"GetNodes": func(s service.NodeService) ([]model.Node, error) {
			return s.GetNodes(context.Background(), []int32{2}, 7)
		},
		"GetNode": func(s service.NodeService) ([]model.Node, error) {
			n, err := s.GetNode(context.Background(), 2, 7)
			if err != nil {
				return nil, err
			}
			return []model.Node{n}, nil
		},
		"GetNodeChildren": func(s service.NodeService) ([]model.Node, error) {
			return s.GetNodeChildren(context.Background(), 1, 7)
		},
		"GetNodeParents": func(s service.NodeService) ([]model.Node, error) {
			return s.GetNodeParents(context.Background(), 2, 7)
		},
		"FindNodesByAntPath": func(s service.NodeService) ([]model.Node, error) {
			_, nodes, err := s.FindNodesByAntPath(context.Background(), "^/a", "", 0, 10, 7)
			return nodes, err
		},
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			props := newStubProps()
			props.inherited = map[string]any{"unit": "°C"}
			svc := inheritedChain(props, &stubNode{
				nodes: []model.Node{newNode(2, model.KindTimeseries, nil)},
			})

			nodes, err := call(svc)
			require.NoError(t, err)
			require.NotEmpty(t, nodes, "the stub must return a node for the test to mean anything")
			assert.Equal(t, []any{"unit"}, mdOf(nodes[0])[model.InheritedKey])
		})
	}
}

// TimeSpan and InheritedProps share one response. Each owns a different marker,
// so neither may discard the other's.
func TestBothDecoratorsKeepTheirOwnMarker(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	props.inherited = map[string]any{"unit": "°C"}

	span, err := service.InitializeTree([]model.NodeTimeSpan{
		{ID: 2, ParentID: 0, LocalFirst: base, LocalLast: base.Add(time.Hour)},
	})
	require.NoError(t, err)

	svc := Chain(
		&stubNode{node: newNode(2, model.KindTimeseries, nil)},
		TimeSpan(TimeSpanConfig{Span: span, SyncWrites: true}),
		InheritedProps(InheritedConfig{
			Props:      props,
			Rules:      []service.PropertyRule{{SourceKind: model.KindFolder}},
			SyncWrites: true,
		}),
	)

	n, err := svc.GetNode(context.Background(), 2, 7)
	require.NoError(t, err)

	md := mdOf(n)
	assert.Equal(t, []any{"unit"}, md[model.InheritedKey])
	assert.Equal(t, []any{"first", "last"}, md[model.AggregatedKey])
	assert.Equal(t, "°C", md["unit"])
	assert.Equal(t, base.Format(time.RFC3339), md[model.DefaultFirstKey])
}

func TestInheritedPropsFailOpenOnPanic(t *testing.T) {
	t.Parallel()

	base := &stubNode{node: newNode(2, model.KindTimeseries, map[string]any{"lat": 1.0})}
	svc := Chain(base, Decorator{
		Name: "boom",
		Enrich: func(_ context.Context, _ *View) {
			panic("enricher exploded")
		},
	})

	// The node is still returned; decoration is additive.
	n, err := svc.GetNode(context.Background(), 2, 7)
	require.NoError(t, err)
	assert.Equal(t, 1.0, mdOf(n)["lat"])
}

func TestUpdateNodeRejectsInheritedMarker(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	fields := map[string]any{
		"metadata": map[string]any{
			"unit":             "V",
			model.InheritedKey: []any{"unit"},
		},
	}
	err := svc.UpdateNode(context.Background(), 2, &fields)

	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrInheritedNotWritable)
	assert.Empty(t, base.calls, "the base service must not be reached")
	assert.Empty(t, props.local, "nothing may be recorded as a local value")
}

func TestUpdateNodeAcceptsOverrideWithoutMarker(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	// The documented way to override an inherited value: send a plain local
	// value with no marker.
	fields := map[string]any{"metadata": map[string]any{"unit": "V"}}
	require.NoError(t, svc.UpdateNode(context.Background(), 2, &fields))

	assert.Contains(t, base.calls, "UpdateNode")
	assert.Equal(t, "V", props.local[2].Map()["unit"])
}

// The aggregated marker is refused with its own error, so the 400 can say
// whether the values came from an ancestor or from a subtree roll-up.
func TestUpdateNodeRejectsAggregatedMarker(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	fields := map[string]any{
		"metadata": map[string]any{
			"first":             "2023-01-01T00:00:00Z",
			model.AggregatedKey: []any{"first"},
		},
	}
	err := svc.UpdateNode(context.Background(), 2, &fields)

	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrAggregatedNotWritable)
	assert.NotErrorIs(t, err, repository.ErrInheritedNotWritable)
	assert.Empty(t, base.calls, "the base service must not be reached")
}

func TestUpdateNodeRejectionDoesNotPartiallyWrite(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	// A legitimate field sits alongside the offending one. The whole body is
	// refused, so display_name is not written either.
	fields := map[string]any{
		"display_name": "new name",
		"metadata": map[string]any{
			model.InheritedKey: []any{"unit"},
		},
	}
	err := svc.UpdateNode(context.Background(), 2, &fields)

	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrInheritedNotWritable)
	assert.Empty(t, base.calls, "no partial write")
}

func TestUpdateNodeIgnoresMarkerOutsideMetadata(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	// A top-level key of the same name is not the marker; only metadata is
	// checked, and the base service decides what to do with other keys.
	fields := map[string]any{model.InheritedKey: []any{"unit"}}
	assert.NoError(t, svc.UpdateNode(context.Background(), 2, &fields))
	assert.Contains(t, base.calls, "UpdateNode")
}

func TestUpdateNodeTracksMoveAndMetadata(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	fields := map[string]any{
		"parent_id": float64(9),
		"metadata":  map[string]any{"unit": "V"},
	}
	require.NoError(t, svc.UpdateNode(context.Background(), 2, &fields))

	assert.Equal(t, int32(9), props.moved[2])
	assert.Equal(t, "V", props.local[2].Map()["unit"])
}

func TestUpdateNodePropagatesBaseError(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{err: assert.AnError}
	svc := inheritedChain(props, base)

	fields := map[string]any{"parent_id": float64(9)}
	err := svc.UpdateNode(context.Background(), 2, &fields)

	assert.ErrorIs(t, err, assert.AnError)
	// A failed move must not be recorded, or the tree would diverge from the
	// database.
	assert.Empty(t, props.moved)
}

func TestUpdateNodeMetadataRejectsInheritedMarker(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	err := svc.UpdateNodeMetadata(context.Background(), 2, map[string]any{
		"unit":             "V",
		model.InheritedKey: []any{"unit"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrInheritedNotWritable)
	assert.Empty(t, base.calls)
}

func TestUpdateNodeMetadataRecordsLocalValue(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	require.NoError(t, svc.UpdateNodeMetadata(context.Background(), 2, map[string]any{"unit": "V"}))

	assert.Contains(t, base.calls, "UpdateNodeMetadata")
	assert.Equal(t, "V", props.local[2].Map()["unit"])
}

func TestCreateNodeRegistersLocalMetadata(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	node := newNode(2, model.KindTimeseries, map[string]any{"unit": "V"})
	require.NoError(t, svc.CreateNode(context.Background(), &node, 7))

	assert.Contains(t, base.calls, "CreateNode")
}

// A create body carrying the marker is refused, matching the update paths, so
// the API behaves the same way no matter which write a client uses.
func TestCreateNodeRejectsInheritedMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		md   map[string]any
	}{
		{
			name: "marker naming a value",
			md:   map[string]any{"unit": "V", model.InheritedKey: []any{"unit"}},
		},
		{
			name: "marker on its own",
			md:   map[string]any{model.InheritedKey: []any{}},
		},
		{
			name: "malformed marker still refused",
			md:   map[string]any{"unit": "V", model.InheritedKey: "unit"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			props := newStubProps()
			base := &stubNode{}
			svc := inheritedChain(props, base)

			node := newNode(2, model.KindTimeseries, tc.md)
			err := svc.CreateNode(context.Background(), &node, 7)

			require.ErrorIs(t, err, repository.ErrInheritedNotWritable)
			assert.NotContains(t, base.calls, "CreateNode",
				"nothing reaches the base service, so nothing can be stored")
		})
	}
}

func TestCreateNodeByPathRejectsInheritedMarker(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	node := newNode(2, model.KindFolder, map[string]any{
		"unit": "V", model.InheritedKey: []any{"unit"},
	})
	err := svc.CreateNodeByPath(context.Background(), &node, 1, "a/b", 7)

	require.ErrorIs(t, err, repository.ErrInheritedNotWritable)
	assert.NotContains(t, base.calls, "CreateNodeByPath")
}

func TestCreateNodeAcceptsPlainLocalMetadata(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	node := newNode(2, model.KindTimeseries, map[string]any{"unit": "V"})
	require.NoError(t, svc.CreateNode(context.Background(), &node, 7))

	assert.Contains(t, base.calls, "CreateNode")
}

func TestDeleteNodesPassesWholeBatch(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	require.NoError(t, svc.DeleteNodesByIDs(context.Background(), []int32{2, 3, 4}))

	// One batch call, so the property service cascades in a single pass.
	assert.Equal(t, []int32{2, 3, 4}, props.deleted)
	assert.Equal(t, []string{"DeleteNodesByIDs"}, base.calls)
}

func TestDeleteNodePassesSingularId(t *testing.T) {
	t.Parallel()

	props := newStubProps()
	base := &stubNode{}
	svc := inheritedChain(props, base)

	require.NoError(t, svc.DeleteNode(context.Background(), 2))
	assert.Equal(t, []int32{2}, props.deleted)
}

func TestInheritedPropsRequiresProps(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		InheritedProps(InheritedConfig{Rules: []service.PropertyRule{{SourceKind: model.KindFolder}}})
	})
}

func TestInheritedPropsRequiresRules(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		InheritedProps(InheritedConfig{Props: newStubProps()})
	})
}

func TestInheritedPropsRefreshRequiresLoader(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		InheritedProps(InheritedConfig{
			Props:        newStubProps(),
			Rules:        []service.PropertyRule{{SourceKind: model.KindFolder}},
			RefreshEvery: time.Minute,
		})
	})
}

func TestInheritedPropsRefreshRebuildsTree(t *testing.T) {
	t.Parallel()

	// A write that bypassed the decorated service, as user sign-up does, is
	// picked up by the next refresh.
	loaders := 0
	loader := func(context.Context) ([]model.Node, error) {
		loaders++
		return []model.Node{
			{ID: 1, Kind: model.KindFolder, Metadata: mustMetadata(t, map[string]any{"unit": "K"})},
		}, nil
	}

	cfg := InheritedConfig{
		Props:        newStubProps(),
		Rules:        []service.PropertyRule{{SourceKind: model.KindFolder}},
		Loader:       loader,
		RefreshEvery: 10 * time.Millisecond,
	}
	// The refresher runs on its own goroutine; wait for the first load rather
	// than sleeping a fixed amount.
	InheritedProps(cfg)

	assert.Eventually(t, func() bool { return loaders > 0 }, 2*time.Second, 5*time.Millisecond)
}

func mustMetadata(t *testing.T, m map[string]any) model.Metadata {
	t.Helper()
	md, err := model.MetadataFromMap(m)
	require.NoError(t, err)
	return md
}

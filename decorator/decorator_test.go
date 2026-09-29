package decorator

import (
	"context"
	"errors"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wrapOnly returns a Decorator that only layers behaviour onto the service and
// never touches the payload.
func wrapOnly(name string, order *[]string) Decorator {
	return Decorator{
		Name: name,
		Wrap: func(next service.NodeService) service.NodeService {
			return &tracer{NodeService: next, name: name, order: order}
		},
	}
}

// tracer is a Wrap-only decorator that records when it is entered.
type tracer struct {
	service.NodeService
	name  string
	order *[]string
}

func (t *tracer) GetNode(ctx context.Context, id int32, userID int32) (model.Node, error) {
	*t.order = append(*t.order, "enter:"+t.name)
	n, err := t.NodeService.GetNode(ctx, id, userID)
	*t.order = append(*t.order, "exit:"+t.name)
	return n, err
}

func TestChain(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the base service when no decorator is given", func(t *testing.T) {
		base := &stubNode{}
		assert.Same(t, service.NodeService(base), Chain(base))
	})

	t.Run("Wrap is applied outside-in in declaration order", func(t *testing.T) {
		var order []string
		base := &stubNode{node: newNode(1, "Folder", nil)}

		svc := Chain(base, wrapOnly("a", &order), wrapOnly("b", &order))
		_, err := svc.GetNode(ctx, 1, 1)

		require.NoError(t, err)
		// The first declared decorator is the outermost layer.
		assert.Equal(t, []string{"enter:a", "enter:b", "exit:b", "exit:a"}, order)
		// The wrapped call reached the base service exactly once.
		assert.Equal(t, []string{"GetNode"}, base.calls)
	})

	t.Run("enrich runs in declaration order and the last writer wins", func(t *testing.T) {
		base := &stubNode{node: newNode(1, "Folder", nil)}
		first := Decorator{Name: "first", Enrich: func(_ context.Context, v *View) {
			v.Metadata()["order"] = "first"
			v.Metadata()["onlyFirst"] = true
		}}
		second := Decorator{Name: "second", Enrich: func(_ context.Context, v *View) {
			v.Metadata()["order"] = "second"
		}}

		svc := Chain(base, first, second)
		n, err := svc.GetNode(ctx, 1, 1)

		require.NoError(t, err)
		md := mdOf(n)
		assert.Equal(t, "second", md["order"])
		assert.Equal(t, true, md["onlyFirst"])
	})

	t.Run("a decorator with no enrich leaves the payload untouched", func(t *testing.T) {
		base := &stubNode{node: newNode(1, "Folder", map[string]any{"unit": "C"})}
		var order []string

		svc := Chain(base, wrapOnly("a", &order))
		n, err := svc.GetNode(ctx, 1, 1)

		require.NoError(t, err)
		assert.Equal(t, map[string]any{"unit": "C"}, mdOf(n))
	})

	t.Run("a decorator with no wrap is transparent", func(t *testing.T) {
		base := &stubNode{node: newNode(1, "Folder", nil)}
		var order []string

		svc := Chain(base, Decorator{
			Name:   "enrich-only",
			Enrich: func(_ context.Context, v *View) { v.Metadata()["k"] = "v" },
		}, wrapOnly("a", &order))

		n, err := svc.GetNode(ctx, 1, 1)
		require.NoError(t, err)
		assert.Equal(t, "v", mdOf(n)["k"])
		assert.Equal(t, []string{"enter:a", "exit:a"}, order)
	})

	t.Run("a selector that rejects the node leaves it undecorated", func(t *testing.T) {
		base := &stubNode{node: newNode(1, "Folder", map[string]any{"unit": "C"})}
		svc := Chain(base, Decorator{
			Name:   "charts-only",
			When:   KindIs("Chart"),
			Enrich: func(_ context.Context, v *View) { v.Metadata()["k"] = "v" },
		})

		n, err := svc.GetNode(ctx, 1, 1)
		require.NoError(t, err)
		// Not even a commit happens, so the payload is byte-for-byte identical.
		assert.Equal(t, map[string]any{"unit": "C"}, mdOf(n))
	})

	t.Run("a panicking enricher does not fail the call", func(t *testing.T) {
		base := &stubNode{node: newNode(1, "Folder", nil)}
		svc := Chain(base, Decorator{
			Name: "boom",
			Enrich: func(context.Context, *View) {
				panic("decorator exploded")
			},
		}, Decorator{
			Name:   "after",
			Enrich: func(_ context.Context, v *View) { v.Metadata()["after"] = true },
		})

		n, err := svc.GetNode(ctx, 1, 1)

		require.NoError(t, err, "enrich is fail-open")
		assert.Equal(t, int32(1), n.ID)
		assert.Equal(t, true, mdOf(n)["after"], "later enrichers still run")
	})

	t.Run("a service error is propagated and skips enrichment", func(t *testing.T) {
		base := &stubNode{err: errors.New("boom")}
		svc := Chain(base, Decorator{
			Name:   "k",
			Enrich: func(_ context.Context, v *View) { v.Metadata()["k"] = "v" },
		})

		_, err := svc.GetNode(ctx, 1, 1)
		require.Error(t, err)

		nodes, err := svc.GetNodes(ctx, nil, 1)
		require.Error(t, err)
		assert.Nil(t, nodes)
	})

	t.Run("every node returning method is enriched", func(t *testing.T) {
		enrich := func(_ context.Context, v *View) { v.Metadata()["seen"] = v.ID() }

		t.Run("GetNode", func(t *testing.T) {
			svc := Chain(&stubNode{node: newNode(7, "Folder", nil)}, Decorator{Enrich: enrich})
			n, err := svc.GetNode(ctx, 7, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 7, mdOf(n)["seen"])
		})

		t.Run("GetNodes", func(t *testing.T) {
			svc := Chain(&stubNode{nodes: []model.Node{newNode(1, "Folder", nil), newNode(2, "Folder", nil)}}, Decorator{Enrich: enrich})
			nodes, err := svc.GetNodes(ctx, nil, 1)
			require.NoError(t, err)
			require.Len(t, nodes, 2)
			assert.EqualValues(t, 1, mdOf(nodes[0])["seen"])
			assert.EqualValues(t, 2, mdOf(nodes[1])["seen"])
		})

		t.Run("GetNodeChildren", func(t *testing.T) {
			svc := Chain(&stubNode{nodes: []model.Node{newNode(3, "Folder", nil)}}, Decorator{Enrich: enrich})
			nodes, err := svc.GetNodeChildren(ctx, 3, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 3, mdOf(nodes[0])["seen"])
		})

		t.Run("GetNodeParents", func(t *testing.T) {
			svc := Chain(&stubNode{nodes: []model.Node{newNode(4, "Folder", nil)}}, Decorator{Enrich: enrich})
			nodes, err := svc.GetNodeParents(ctx, 4, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 4, mdOf(nodes[0])["seen"])
		})

		t.Run("FindNodesByAntPath", func(t *testing.T) {
			base := &stubNode{
				nodes: []model.Node{newNode(5, "Timeseries", nil)},
				total: 1,
			}
			svc := Chain(base, Decorator{Enrich: enrich})

			total, nodes, err := svc.FindNodesByAntPath(ctx, "**", "Timeseries", 0, 10, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			require.Len(t, nodes, 1)
			assert.EqualValues(t, 5, mdOf(nodes[0])["seen"])
		})
	})
}

func TestNames(t *testing.T) {
	decs := []Decorator{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	assert.Equal(t, []string{"a", "b", "c"}, Names(decs))
}

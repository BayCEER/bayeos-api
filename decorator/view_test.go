package decorator

import (
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewCommit(t *testing.T) {
	t.Run("writes metadata back onto the node", func(t *testing.T) {
		n := newNode(1, "Timeseries", map[string]any{"unit": "C"})
		v := NewView(&n)

		v.Metadata()["first"] = "2023-01-01T00:00:00Z"
		v.Commit()

		assert.Equal(t, map[string]any{
			"unit":  "C",
			"first": "2023-01-01T00:00:00Z",
		}, mdOf(n))
	})

	t.Run("preserves keys written before the view was created", func(t *testing.T) {
		n := newNode(1, "Timeseries", map[string]any{"unit": "C", "resolution": 3600})
		v := NewView(&n)

		v.Metadata()["last"] = "2023-01-02T00:00:00Z"
		v.Commit()

		md := mdOf(n)
		assert.Equal(t, "C", md["unit"])
		assert.EqualValues(t, 3600, md["resolution"])
		assert.Equal(t, "2023-01-02T00:00:00Z", md["last"])
	})

	t.Run("empty metadata normalises to an empty object", func(t *testing.T) {
		n := newNode(1, "Folder", nil)
		v := NewView(&n)

		v.Metadata()["k"] = "v"
		v.Commit()

		assert.Equal(t, map[string]any{"k": "v"}, mdOf(n))
	})

	t.Run("commit without a node is a no-op", func(t *testing.T) {
		v := NewView(nil)
		v.Metadata()["k"] = "v"
		assert.NotPanics(t, v.Commit)
		assert.Nil(t, v.Node)
	})

	t.Run("accessors are safe on an empty view", func(t *testing.T) {
		v := NewView(nil)
		assert.Equal(t, int32(0), v.ID())
		assert.Equal(t, "", v.Kind())
		assert.Equal(t, "", v.DisplayName())
		assert.NotNil(t, v.Metadata())
	})
}

func TestSelector(t *testing.T) {
	folder := NewView(&model.Node{ID: 1, Kind: "Folder"})
	series := NewView(&model.Node{ID: 2, Kind: "Timeseries"})

	t.Run("Always and Never", func(t *testing.T) {
		assert.True(t, Always()(folder))
		assert.False(t, Never()(folder))
	})

	t.Run("KindIs matches listed kinds", func(t *testing.T) {
		sel := KindIs("Timeseries")
		assert.False(t, sel(folder))
		assert.True(t, sel(series))
	})

	t.Run("KindIs with no kinds matches everything", func(t *testing.T) {
		// An unconfigured kind filter must not silently disable a decorator.
		sel := KindIs()
		assert.True(t, sel(folder))
		assert.True(t, sel(series))
	})

	t.Run("Not inverts", func(t *testing.T) {
		sel := Not(KindIs("Timeseries"))
		assert.True(t, sel(folder))
		assert.False(t, sel(series))
	})

	t.Run("Not of nil is Never", func(t *testing.T) {
		assert.False(t, Not(nil)(folder))
	})

	t.Run("All requires every selector", func(t *testing.T) {
		sel := All(KindIs("Folder", "Timeseries"), Not(Never()))
		assert.True(t, sel(folder))
		assert.True(t, sel(series))

		assert.False(t, All(KindIs("Folder"), KindIs("Timeseries"))(folder))
	})

	t.Run("Any requires one selector", func(t *testing.T) {
		sel := Any(KindIs("Folder"), KindIs("Timeseries"))
		assert.True(t, sel(folder))
		assert.True(t, sel(series))
		assert.False(t, Any(KindIs("Chart"))(folder))
	})

	t.Run("nil selector matches everything", func(t *testing.T) {
		require.True(t, matches(nil, folder))
	})
}

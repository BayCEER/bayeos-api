package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubPlugin struct {
	name string
}

func (s *stubPlugin) Name() string           { return s.name }
func (s *stubPlugin) Register(h *Host) error { return nil }

func TestRegistryRegistrationOrder(t *testing.T) {
	Reset()
	defer Reset()

	Register(&stubPlugin{name: "a"})
	Register(&stubPlugin{name: "b"})

	all := All()
	require.Len(t, all, 2)
	require.Equal(t, "a", all[0].Name())
	require.Equal(t, "b", all[1].Name())
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	Reset()
	defer Reset()

	Register(&stubPlugin{name: "dup"})
	require.Panics(t, func() {
		Register(&stubPlugin{name: "dup"})
	})
}

func TestRegistryRejectsUnnamed(t *testing.T) {
	Reset()
	defer Reset()

	require.Panics(t, func() {
		Register(&stubPlugin{name: ""})
	})
}

func TestHostNodeDeleteHook(t *testing.T) {
	h := &Host{}
	var got [][]int32
	h.AddNodeDeleteHook(func(ctx context.Context, nodeIDs []int32) error {
		got = append(got, nodeIDs)
		return nil
	})
	h.AddNodeDeleteHook(func(ctx context.Context, nodeIDs []int32) error {
		shifted := make([]int32, len(nodeIDs))
		for i, id := range nodeIDs {
			shifted[i] = id + 100
		}
		got = append(got, shifted)
		return nil
	})

	hooks := h.NodeDeleteHooks()
	require.Len(t, hooks, 2)

	for _, fn := range hooks {
		require.NoError(t, fn(context.Background(), []int32{7, 8}))
	}

	require.Equal(t, [][]int32{{7, 8}, {107, 108}}, got)
}

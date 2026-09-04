package handler

import (
	"context"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/plugin"
	"github.com/stretchr/testify/require"
)

func TestRunNodeDeleteHooks(t *testing.T) {
	var got [][]int32
	h := &Handler{}
	h.nodeDeleteHooks = []plugin.NodeDeleteHookFunc{
		func(ctx context.Context, nodeIDs []int32) error {
			got = append(got, nodeIDs)
			return nil
		},
		func(ctx context.Context, nodeIDs []int32) error {
			got = append(got, nodeIDs) // second hook also runs, order preserved
			return nil
		},
	}

	h.runNodeDeleteHooks(context.Background(), []int32{10, 11, 12})
	require.Equal(t, [][]int32{{10, 11, 12}, {10, 11, 12}}, got)
}

func TestRunNodeDeleteHooksEmpty(t *testing.T) {
	h := &Handler{}
	h.runNodeDeleteHooks(context.Background(), []int32{13}) // must not panic
}
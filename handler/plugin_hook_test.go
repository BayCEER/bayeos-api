package handler

import (
	"context"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/plugin"
	"github.com/stretchr/testify/require"
)

func TestRunNodeDeleteHooks(t *testing.T) {
	var got []int32
	h := &Handler{}
	h.nodeDeleteHooks = []plugin.NodeDeleteHookFunc{
		func(ctx context.Context, nodeID int32) error {
			got = append(got, nodeID)
			return nil
		},
		func(ctx context.Context, nodeID int32) error {
			got = append(got, nodeID) // second hook also runs, order preserved
			return nil
		},
	}

	h.runNodeDeleteHooks(context.Background(), 13)
	require.Equal(t, []int32{13, 13}, got)
}

func TestRunNodeDeleteHooksEmpty(t *testing.T) {
	h := &Handler{}
	h.runNodeDeleteHooks(context.Background(), 13) // must not panic
}
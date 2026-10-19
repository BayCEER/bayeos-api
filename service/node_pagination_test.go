package service

import (
	"context"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hits(n int) []model.Node {
	out := make([]model.Node, n)
	for i := range out {
		out[i] = model.Node{ID: int32(i + 1), DisplayName: "n"}
	}
	return out
}

func idsOf(nodes []model.Node) []int32 {
	out := make([]int32, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}

// paginateHits replaces the per-node permission loop for a super user, so it
// has to agree with that loop on which hits land in the window. This checks it
// against a literal transcription of the loop it stands in for.
func TestPaginateHitsMatchesPerNodeLoop(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		total int
		skip  int
		limit int
	}{
		{name: "first page", total: 5, skip: 0, limit: 3},
		{name: "second page", total: 5, skip: 3, limit: 3},
		{name: "limit past the end", total: 3, skip: 0, limit: 10},
		{name: "window exactly at the end", total: 4, skip: 2, limit: 2},
		{name: "skip past the end", total: 3, skip: 5, limit: 2},
		{name: "skip equals length", total: 3, skip: 3, limit: 2},
		{name: "no hits", total: 0, skip: 0, limit: 10},
		{name: "zero limit", total: 5, skip: 0, limit: 0},
		{name: "negative limit", total: 5, skip: 0, limit: -1},
		{name: "negative skip", total: 5, skip: -2, limit: 3},
		{name: "negative skip and limit", total: 5, skip: -1, limit: -1},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The loop as it stands in FindNodesByAntPath, with every node
			// reported readable and full permission.
			var want []model.Node
			for i, hit := range hits(tt.total) {
				hit.Permission = model.FULL_PERMISSION
				if tt.skip <= i && i < tt.skip+tt.limit {
					want = append(want, hit)
				}
			}
			wantIDs := idsOf(want)
			if want == nil {
				wantIDs = []int32{}
			}

			got := paginateHits(hits(tt.total), tt.skip, tt.limit)
			assert.Equal(t, wantIDs, idsOf(got))
		})
	}
}

// fixedAccess is a model.NodeAccess that reports the same permission for every
// node, so readableNodes can be exercised without a repository.
type fixedAccess struct {
	perm uint8
	err  error
}

func (a fixedAccess) Permission(context.Context, int32) (uint8, error) { return a.perm, a.err }

// readableNodes is what stamps permission now, so a subject holding full
// permission must see every hit annotated with it.
func TestReadableNodesStampsPermission(t *testing.T) {
	t.Parallel()

	got, err := (&nodeService{}).readableNodes(context.Background(), hits(3), fixedAccess{perm: model.FULL_PERMISSION})
	require.NoError(t, err)
	require.Len(t, got, 3)
	for i, n := range got {
		assert.Equal(t, model.FULL_PERMISSION, n.Permission, "hit %d should carry full permission", i)
	}
}

// A subject that may not read a node must not see it at all.
func TestReadableNodesDropsUnreadable(t *testing.T) {
	t.Parallel()

	got, err := (&nodeService{}).readableNodes(context.Background(), hits(3), fixedAccess{perm: 0})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// The returned window aliases the input slice, so mutating a hit must not be
// able to reach the caller's own copy of the slice header.
func TestPaginateHitsDoesNotMutateInputSlice(t *testing.T) {
	t.Parallel()

	in := hits(4)
	before := len(in)
	paginateHits(in, 1, 2)
	assert.Len(t, in, before, "input slice should be unchanged")
}
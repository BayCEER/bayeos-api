package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createFolder creates a folder with the given metadata and returns it.
func createFolder(t *testing.T, token string, parentID int32, name string, md map[string]any) model.Node {
	t.Helper()

	metadata, err := model.MetadataFromMap(md)
	require.NoError(t, err)

	req, err := NewJWTRequest("POST", "/bayeos-api/nodes", model.Node{
		DisplayName: name,
		ParentID:    parentID,
		Kind:        model.KindFolder,
		Metadata:    metadata,
	}, token)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var out model.Node
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

// getNode reads a node and returns its raw metadata map.
func getNode(t *testing.T, token string, id int32) map[string]any {
	t.Helper()

	req, err := NewJWTRequest("GET", fmt.Sprintf("/bayeos-api/nodes/%d", id), nil, token)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var out model.Node
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out.Metadata.Map()
}

// patchNode sends a PATCH body and returns the status code.
func patchNode(t *testing.T, token string, id int32, body []byte) int {
	t.Helper()

	req, err := NewJWTReaderRequest("PATCH", fmt.Sprintf("/bayeos-api/nodes/%d", id), bytes.NewBuffer(body), token)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

// TestInheritedMetadataIsNotPersistedOnRoundTrip is the regression test for the
// hazard that motivated rejecting the marker: a client that echoes a node read
// back must not turn inherited values into stored local ones.
//
// Without the rejection, PATCHing the GET body would run
// "metadata = metadata || $1::jsonb" and freeze the inherited unit on the child
// permanently, after which a change to the parent would stop propagating.
func TestInheritedMetadataIsNotPersistedOnRoundTrip(t *testing.T) {
	token, err := SignUp("roundtrip", "roundtrippass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	parent := createFolder(t, token, user.HomeNodeID, "RTParent", map[string]any{"unit": "°C"})
	child := createFolder(t, token, parent.ID, "RTChild", nil)

	// The child reports the parent's unit, marked as inherited.
	md := getNode(t, token, child.ID)
	assert.Equal(t, "°C", md["unit"])
	assert.Contains(t, md, model.InheritedKey)

	// Echo the body back unchanged, exactly as a naive client would.
	echo := fmt.Sprintf(`{"metadata":%s}`, mustJSON(t, md))
	assert.Equal(t, http.StatusBadRequest, patchNode(t, token, child.ID, []byte(echo)),
		"a body carrying the inherited marker is rejected")

	// The stored metadata is untouched: neither the marker nor the inherited
	// value became local. The repository is read directly, because every API
	// read goes through the decorator and would show the derived value.
	stored, err := sqlRepo.FindNodeByID(context.Background(), child.ID)
	require.NoError(t, err)
	storedMD := stored.Metadata.Map()
	assert.NotContains(t, storedMD, "unit", "the child's own metadata must stay empty")
	assert.NotContains(t, storedMD, model.InheritedKey, "the marker must never be stored")

	// And the value is still inherited, not frozen.
	assert.Equal(t, "°C", getNode(t, token, child.ID)["unit"])
}

// TestAggregatedMarkerIsRejectedOverHTTP is the span counterpart of the round
// trip. The handler test suite has no QuestDB connection, so the decorator never
// produces an _aggregated marker here; the body is sent by hand, which is what a
// client echoing a decorated node from another environment would do.
func TestAggregatedMarkerIsRejectedOverHTTP(t *testing.T) {
	token, err := SignUp("aggrecho", "aggrechopass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	child := createFolder(t, token, user.HomeNodeID, "AEChild", map[string]any{"own": "x"})

	echo := fmt.Sprintf(
		`{"metadata":{"own":"x","first":"2020-01-01T00:00:00Z","last":"2020-01-02T00:00:00Z",%q:["first","last"]}}`,
		model.AggregatedKey)
	assert.Equal(t, http.StatusBadRequest, patchNode(t, token, child.ID, []byte(echo)),
		"a body carrying the aggregated marker is rejected")

	stored, err := sqlRepo.FindNodeByID(context.Background(), child.ID)
	require.NoError(t, err)
	md := stored.Metadata.Map()
	assert.NotContains(t, md, model.AggregatedKey)
	assert.NotContains(t, md, "first")
	assert.NotContains(t, md, "last")
	assert.Equal(t, "x", md["own"], "the local value is untouched")
}

// TestSpanKeysAreDroppedOverHTTPWithoutAMarker covers the unconditional strip.
// The write is accepted, but the fabricated span is not kept, so a later read
// cannot serve it back as if it were measured.
func TestSpanKeysAreDroppedOverHTTPWithoutAMarker(t *testing.T) {
	token, err := SignUp("aggrebare", "aggrebarepass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	child := createFolder(t, token, user.HomeNodeID, "ABChild", map[string]any{"own": "x"})

	body := `{"metadata":{"own":"x","first":"2020-01-01T00:00:00Z","last":"2020-01-02T00:00:00Z"}}`
	require.Equal(t, http.StatusOK, patchNode(t, token, child.ID, []byte(body)),
		"there is no marker, so the write itself is legitimate")

	stored, err := sqlRepo.FindNodeByID(context.Background(), child.ID)
	require.NoError(t, err)
	md := stored.Metadata.Map()
	assert.Equal(t, "x", md["own"])
	assert.NotContains(t, md, "first")
	assert.NotContains(t, md, "last")
}

// TestRepositoryOmitsInheritedMarkerWithoutTheDecorator pins the storage-layer
// guarantee on its own. The other round-trip tests rely on the decorator
// rejecting the body, so they would still pass if the repository stopped
// sanitising. Here the decorated body is written straight through the
// repository, with no decorator in the path at all.
//
// This is the case the marker could otherwise corrupt: "metadata = metadata ||
// $1::jsonb" would store the inherited unit on the child, freezing it so a later
// change to the parent never arrives.
func TestRepositoryOmitsInheritedMarkerWithoutTheDecorator(t *testing.T) {
	token, err := SignUp("repostrip", "repostrippass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	parent := createFolder(t, token, user.HomeNodeID, "RSPParent", map[string]any{"unit": "K"})
	child := createFolder(t, token, parent.ID, "RSPChild", nil)

	// A decorated read: the unit is present and marked as inherited.
	echoed := getNode(t, token, child.ID)
	require.Equal(t, "K", echoed["unit"])
	require.Contains(t, echoed, model.InheritedKey)

	// Replay it as the repository sees a write, bypassing every decorator.
	metadata, err := model.MetadataFromMap(echoed)
	require.NoError(t, err)
	require.NoError(t, sqlRepo.MergeNodeMetadata(context.Background(), child.ID, metadata))

	stored, err := sqlRepo.FindNodeByID(context.Background(), child.ID)
	require.NoError(t, err)
	storedMD := stored.Metadata.Map()
	assert.NotContains(t, storedMD, model.InheritedKey, "the repository must not store the marker")
	assert.NotContains(t, storedMD, "unit",
		"the repository must not store a value the marker says came from an ancestor")

	// Values the marker did not name are still written, so a legitimate partial
	// update is not swallowed by the sanitising.
	require.NoError(t, sqlRepo.MergeNodeMetadata(context.Background(), child.ID,
		mustMetadata(t, map[string]any{"own": "kept", "_inherited": []any{"unit"}})))

	stored, err = sqlRepo.FindNodeByID(context.Background(), child.ID)
	require.NoError(t, err)
	storedMD = stored.Metadata.Map()
	assert.Equal(t, "kept", storedMD["own"], "an unnamed local value is still merged")
	assert.NotContains(t, storedMD, "unit")

	// Finally, inheritance still works: the parent change reaches the child.
	require.Equal(t, http.StatusOK,
		patchNode(t, token, parent.ID, []byte(`{"metadata":{"unit":"°C"}}`)))
	assert.Equal(t, "°C", getNode(t, token, child.ID)["unit"],
		"the replayed echo must not have frozen the child's inherited value")
}

// TestCreateNodeOmitsInheritedMarker covers the insert path. A node is created
// with metadata that names itself as inherited; the marker and the value it
// names must not reach the column.
func TestCreateNodeOmitsInheritedMarker(t *testing.T) {
	token, err := SignUp("createstrip", "createstrippass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	n := model.Node{
		DisplayName: "CSChild",
		ParentID:    user.HomeNodeID,
		Kind:        model.KindFolder,
		Metadata:    mustMetadata(t, map[string]any{"unit": "°C", model.InheritedKey: []any{"unit"}}),
	}
	require.NoError(t, sqlRepo.CreateNode(context.Background(), &n))

	stored, err := sqlRepo.FindNodeByID(context.Background(), n.ID)
	require.NoError(t, err)
	assert.NotContains(t, stored.Metadata.Map(), model.InheritedKey,
		"the insert path must not store the marker")
	assert.NotContains(t, stored.Metadata.Map(), "unit",
		"the insert path must not store a value the marker names")
}

// TestUpdateNodeOmitsInheritedMarker covers the full-document update path,
// which replaces metadata wholesale rather than merging it.
func TestUpdateNodeOmitsInheritedMarker(t *testing.T) {
	token, err := SignUp("updatestrip", "updatestrippass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	created := createFolder(t, token, user.HomeNodeID, "USChild", map[string]any{"own": "x"})
	stored, err := sqlRepo.FindNodeByID(context.Background(), created.ID)
	require.NoError(t, err)

	stored.Metadata = mustMetadata(t, map[string]any{
		"own": "x", "unit": "°C", model.InheritedKey: []any{"unit"},
	})
	require.NoError(t, sqlRepo.UpdateNode(context.Background(), &stored))

	reloaded, err := sqlRepo.FindNodeByID(context.Background(), created.ID)
	require.NoError(t, err)
	reloadedMD := reloaded.Metadata.Map()
	assert.Equal(t, "x", reloadedMD["own"], "unnamed local values survive")
	assert.NotContains(t, reloadedMD, model.InheritedKey)
	assert.NotContains(t, reloadedMD, "unit")
}

// mustMetadata builds a Metadata or fails the test.
func mustMetadata(t *testing.T, md map[string]any) model.Metadata {
	t.Helper()
	metadata, err := model.MetadataFromMap(md)
	require.NoError(t, err)
	return metadata
}

// TestSpanKeysAreNeverStored covers the stronger guarantee on the time span keys.
// They are dropped whether or not a marker names them, so a client cannot pin a
// fabricated span to a node by writing the values directly.
func TestSpanKeysAreNeverStored(t *testing.T) {
	token, err := SignUp("spanstrip", "spanstrippass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	n := model.Node{
		DisplayName: "SSChild",
		ParentID:    user.HomeNodeID,
		Kind:        model.KindFolder,
		Metadata: mustMetadata(t, map[string]any{
			"own":               "x",
			"first":             "2020-01-01T00:00:00Z",
			"last":              "2020-01-02T00:00:00Z",
			model.AggregatedKey: []any{"first", "last"},
		}),
	}
	require.NoError(t, sqlRepo.CreateNode(context.Background(), &n))

	stored, err := sqlRepo.FindNodeByID(context.Background(), n.ID)
	require.NoError(t, err)
	md := stored.Metadata.Map()
	assert.Equal(t, "x", md["own"], "unnamed local values survive")
	assert.NotContains(t, md, model.AggregatedKey, "the insert path must not store the marker")
	assert.NotContains(t, md, "first")
	assert.NotContains(t, md, "last")
}

// TestSpanKeysAreDroppedWithoutAMarker is the case a marker-driven strip would
// miss: no marker, just the raw keys.
func TestSpanKeysAreDroppedWithoutAMarker(t *testing.T) {
	token, err := SignUp("spanbare", "spanbarepass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	n := model.Node{
		DisplayName: "SBChild",
		ParentID:    user.HomeNodeID,
		Kind:        model.KindFolder,
		Metadata: mustMetadata(t, map[string]any{
			"own":   "x",
			"first": "2020-01-01T00:00:00Z",
		}),
	}
	require.NoError(t, sqlRepo.CreateNode(context.Background(), &n))

	stored, err := sqlRepo.FindNodeByID(context.Background(), n.ID)
	require.NoError(t, err)
	md := stored.Metadata.Map()
	assert.Equal(t, "x", md["own"])
	assert.NotContains(t, md, "first", "a hand-written span must not survive")
}

// TestMergeNodeMetadataDropsSpanKeys covers the additive merge path, which is
// the one a client reaches when it sends a metadata patch.
func TestMergeNodeMetadataDropsSpanKeys(t *testing.T) {
	token, err := SignUp("spanmerge", "spanmergepass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	created := createFolder(t, token, user.HomeNodeID, "SMChild", map[string]any{"own": "x"})

	require.NoError(t, sqlRepo.MergeNodeMetadata(context.Background(), created.ID, mustMetadata(t, map[string]any{
		"unit":              "V",
		"last":              "2020-01-02T00:00:00Z",
		model.AggregatedKey: []any{"last"},
	})))

	reloaded, err := sqlRepo.FindNodeByID(context.Background(), created.ID)
	require.NoError(t, err)
	md := reloaded.Metadata.Map()
	assert.Equal(t, "V", md["unit"], "a genuine local value is merged")
	assert.Equal(t, "x", md["own"], "and existing values are left alone")
	assert.NotContains(t, md, model.AggregatedKey)
	assert.NotContains(t, md, "last")
}

// TestInheritedValueFollowsParentAfterRejectedRoundTrip is the strongest form of
// the regression: after a rejected echo, changing the parent must still reach
// the child. If the echo had been persisted, the child would be frozen on the
// old value.
func TestInheritedValueFollowsParentAfterRejectedRoundTrip(t *testing.T) {
	token, err := SignUp("frozen", "frozenpass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	parent := createFolder(t, token, user.HomeNodeID, "FZParent", map[string]any{"unit": "K"})
	child := createFolder(t, token, parent.ID, "FZChild", nil)

	// Read, then echo the body back: rejected.
	md := getNode(t, token, child.ID)
	require.Equal(t, "K", md["unit"])
	echo := fmt.Sprintf(`{"metadata":%s}`, mustJSON(t, md))
	require.Equal(t, http.StatusBadRequest, patchNode(t, token, child.ID, []byte(echo)))

	// The parent changes; the child must follow.
	require.Equal(t, http.StatusOK,
		patchNode(t, token, parent.ID, []byte(`{"metadata":{"unit":"°C"}}`)))

	assert.Equal(t, "°C", getNode(t, token, child.ID)["unit"],
		"the rejected echo must not have frozen the child's inherited value")
}

// TestInheritedMetadataFollowsParentChanges proves the inherited value is
// resolved per read rather than frozen at the time of the first read.
func TestInheritedMetadataFollowsParentChanges(t *testing.T) {
	token, err := SignUp("propagate", "propagatepass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	parent := createFolder(t, token, user.HomeNodeID, "PCParent", map[string]any{"unit": "K"})
	child := createFolder(t, token, parent.ID, "PCChild", nil)

	assert.Equal(t, "K", getNode(t, token, child.ID)["unit"])

	// Change the parent, then read the child again.
	require.Equal(t, http.StatusOK,
		patchNode(t, token, parent.ID, []byte(`{"metadata":{"unit":"°C"}}`)))

	assert.Equal(t, "°C", getNode(t, token, child.ID)["unit"],
		"the child must follow the new parent value")
}

// TestLocalOverrideWinsOverInherited covers the local-wins rule end to end.
func TestLocalOverrideWinsOverInherited(t *testing.T) {
	token, err := SignUp("override", "overridepass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	parent := createFolder(t, token, user.HomeNodeID, "OWParent", map[string]any{"unit": "K"})
	child := createFolder(t, token, parent.ID, "OWChild", map[string]any{"unit": "V"})

	md := getNode(t, token, child.ID)
	assert.Equal(t, "V", md["unit"], "a local value is not overwritten")
}

// TestNearestAncestorWins covers precedence through a three-level chain.
func TestNearestAncestorWins(t *testing.T) {
	token, err := SignUp("nearest", "nearestpass")
	require.NoError(t, err)
	user, err := CurrentUser(token)
	require.NoError(t, err)

	grandparent := createFolder(t, token, user.HomeNodeID, "NAParent", map[string]any{"unit": "K"})
	parent := createFolder(t, token, grandparent.ID, "NAMid", map[string]any{"unit": "°C"})
	child := createFolder(t, token, parent.ID, "NAChild", nil)

	assert.Equal(t, "°C", getNode(t, token, child.ID)["unit"])
}

// mustJSON marshals v or fails the test.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

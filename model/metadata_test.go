package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mdOf(t *testing.T, doc string) Metadata {
	t.Helper()
	md, err := MetadataFromMap(map[string]any{})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(doc), &md))
	return md
}

func TestStripComputedDropsMarkersAndDerivedKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "drops the marker and every key it names",
			in:   `{"unit":"°C","lat":1,"_inherited":["unit","lat"]}`,
			want: `{}`,
		},
		{
			name: "keeps values the marker does not name",
			in:   `{"unit":"°C","own":"x","_inherited":["unit"]}`,
			want: `{"own":"x"}`,
		},
		{
			name: "leaves a document without a marker untouched",
			in:   `{"unit":"V","nested":{"a":1},"list":[1,2]}`,
			want: `{"unit":"V","nested":{"a":1},"list":[1,2]}`,
		},
		{
			name: "a derived key named but absent is a no-op",
			in:   `{"own":"x","_inherited":["gone"]}`,
			want: `{"own":"x"}`,
		},
		{
			name: "a malformed marker still gets dropped",
			in:   `{"unit":"V","_inherited":"unit"}`,
			want: `{"unit":"V"}`,
		},
		{
			name: "non-string entries are ignored, not fatal",
			in:   `{"unit":"V","own":"x","_inherited":[7,null,"unit"]}`,
			want: `{"own":"x"}`,
		},
		{
			name: "an empty marker is still removed",
			in:   `{"unit":"V","_inherited":[]}`,
			want: `{"unit":"V"}`,
		},
		{
			name: "the aggregated marker is stripped the same way",
			in:   `{"first":"a","last":"b","own":"x","_aggregated":["first","last"]}`,
			want: `{"own":"x"}`,
		},
		{
			name: "both markers are honoured together",
			in:   `{"unit":"°C","lat":1,"own":"x","_inherited":["unit"],"_aggregated":["lat"]}`,
			want: `{"own":"x"}`,
		},
		{
			name: "a key named by both markers is dropped once",
			in:   `{"unit":"°C","_inherited":["unit"],"_aggregated":["unit"]}`,
			want: `{}`,
		},
		{
			name: "an empty document stays empty",
			in:   `{}`,
			want: `{}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out := StripComputed(mdOf(t, tc.in))

			for _, marker := range ComputedMarkers {
				assert.NotContains(t, out.Map(), marker, "the marker must never survive")
			}
			assert.JSONEq(t, tc.want, string(out.RawMessage))
		})
	}
}

// A marker names keys whose value came from somewhere other than the node, so
// dropping the marker while keeping the value would leave a derived value stored
// as if it were local. That is the exact corruption the strip exists to prevent.
func TestStripComputedRemovesDerivedValuesNotJustTheMarker(t *testing.T) {
	t.Parallel()

	out := StripComputed(mdOf(t, `{"unit":"°C","_inherited":["unit"]}`))

	assert.Empty(t, out.Map(), "the derived value must be dropped along with the marker")
}

// The span keys are dropped even when no marker names them. A client that
// hand-writes "first"/"last" without echoing a marker would otherwise be able to
// pin a fabricated time span to a node and make it look measured.
func TestStripComputedAlwaysDropsSpanKeysWithoutAMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a bare first/last pair is removed",
			in:   `{"first":"2024-01-01T00:00:00Z","last":"2024-01-02T00:00:00Z"}`,
			want: `{}`,
		},
		{
			name: "they are removed without being named",
			in:   `{"first":"a","own":"x","_inherited":["first"]}`,
			want: `{"own":"x"}`,
		},
		{
			name: "only the span keys go",
			in:   `{"first":"a","last":"b","own":"x"}`,
			want: `{"own":"x"}`,
		},
		{
			name: "a falsy span value is still dropped",
			in:   `{"first":"","last":null,"own":"x"}`,
			want: `{"own":"x"}`,
		},
		{
			name: "custom span keys are marker-protected, not always dropped",
			in:   `{"ts_from":"a","own":"x"}`,
			want: `{"ts_from":"a","own":"x"}`,
		},
		{
			name: "but a custom key is dropped when the marker names it",
			in:   `{"ts_from":"a","own":"x","_aggregated":["ts_from"]}`,
			want: `{"own":"x"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out := StripComputed(mdOf(t, tc.in))

			assert.JSONEq(t, tc.want, string(out.RawMessage))
		})
	}
}

// The input document must be left intact, because the caller keeps using it for
// the response it is building.
func TestStripComputedDoesNotMutateItsInput(t *testing.T) {
	t.Parallel()

	in := mdOf(t, `{"unit":"°C","own":"x","_inherited":["unit"]}`)

	StripComputed(in)

	assert.JSONEq(t, `{"unit":"°C","own":"x","_inherited":["unit"]}`, string(in.RawMessage),
		"the source document is still needed for the response")
}

func mapOf(t *testing.T, doc string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(doc), &out))
	return out
}

func TestAddMarkerKeyBuildsSortedUniqueLists(t *testing.T) {
	t.Parallel()

	t.Run("adds to a document with no marker", func(t *testing.T) {
		t.Parallel()

		md := mapOf(t, `{"own":"x"}`)
		AddMarkerKey(md, InheritedKey, "unit")

		assert.Equal(t, map[string]any{"own": "x", "_inherited": []any{"unit"}}, md)
	})

	t.Run("merges with what another decorator already listed", func(t *testing.T) {
		t.Parallel()

		md := mapOf(t, `{"_inherited":["lat"]}`)
		AddMarkerKey(md, InheritedKey, "unit")

		assert.Equal(t, map[string]any{"_inherited": []any{"lat", "unit"}}, md,
			"a second decorator must not discard the first one's list")
	})

	t.Run("keeps the list sorted", func(t *testing.T) {
		t.Parallel()

		md := mapOf(t, `{}`)
		AddMarkerKey(md, InheritedKey, "unit")
		AddMarkerKey(md, InheritedKey, "lat")
		AddMarkerKey(md, InheritedKey, "alt")

		assert.Equal(t, []any{"alt", "lat", "unit"}, md["_inherited"],
			"a stable list keeps responses comparable")
	})

	t.Run("does not list a key twice", func(t *testing.T) {
		t.Parallel()

		md := mapOf(t, `{"_inherited":["unit"]}`)
		AddMarkerKey(md, InheritedKey, "unit")
		AddMarkerKey(md, InheritedKey, "unit")

		assert.Equal(t, []any{"unit"}, md["_inherited"])
	})

	t.Run("a marker that named nothing is left out", func(t *testing.T) {
		t.Parallel()

		md := mapOf(t, `{"own":"x"}`)
		AddMarkerKey(md, InheritedKey, "")

		assert.NotContains(t, md, InheritedKey,
			"an empty marker advertises a guarantee it does not make")
	})

	t.Run("a malformed marker is repaired rather than trusted", func(t *testing.T) {
		t.Parallel()

		md := mapOf(t, `{"_inherited":"unit"}`)
		AddMarkerKey(md, InheritedKey, "unit")

		assert.Equal(t, []any{"unit"}, md["_inherited"])
	})

	t.Run("a nil document is left alone", func(t *testing.T) {
		t.Parallel()

		assert.NotPanics(t, func() { AddMarkerKey(nil, InheritedKey, "unit") })
	})
}

// MergeMetadata must agree with Postgres' jsonb ||, because the service mirror
// and the column are updated by different code and must not drift apart.
func TestMergeMetadataMatchesJsonbConcat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		base  string
		patch string
		want  string
	}{
		{
			name:  "a patch adds a key",
			base:  `{"own":"x"}`,
			patch: `{"unit":"V"}`,
			want:  `{"own":"x","unit":"V"}`,
		},
		{
			name:  "a patch overwrites the key it names",
			base:  `{"unit":"V"}`,
			patch: `{"unit":"°C"}`,
			want:  `{"unit":"°C"}`,
		},
		{
			name:  "a partial patch leaves other keys alone",
			base:  `{"own":"x","unit":"V","lat":1}`,
			patch: `{"unit":"°C"}`,
			want:  `{"own":"x","unit":"°C","lat":1}`,
		},
		{
			name:  "nested objects merge rather than replace",
			base:  `{"opts":{"a":1,"b":2}}`,
			patch: `{"opts":{"b":3,"c":4}}`,
			want:  `{"opts":{"a":1,"b":3,"c":4}}`,
		},
		{
			name:  "an object replaces a scalar",
			base:  `{"opts":"x"}`,
			patch: `{"opts":{"a":1}}`,
			want:  `{"opts":{"a":1}}`,
		},
		{
			name:  "a scalar replaces an object",
			base:  `{"opts":{"a":1}}`,
			patch: `{"opts":"x"}`,
			want:  `{"opts":"x"}`,
		},
		{
			name:  "an array is replaced whole, not concatenated",
			base:  `{"tags":["a","b"]}`,
			patch: `{"tags":["c"]}`,
			want:  `{"tags":["c"]}`,
		},
		{
			name:  "a null stores a null and does not delete the key",
			base:  `{"own":"x"}`,
			patch: `{"own":null}`,
			want:  `{"own":null}`,
		},
		{
			name:  "an empty patch changes nothing",
			base:  `{"own":"x"}`,
			patch: `{}`,
			want:  `{"own":"x"}`,
		},
		{
			name:  "an empty base takes the patch",
			base:  `{}`,
			patch: `{"own":"x"}`,
			want:  `{"own":"x"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out := MergeMetadata(mdOf(t, tc.base), mdOf(t, tc.patch))

			assert.JSONEq(t, tc.want, string(out.RawMessage))
		})
	}
}

func TestMergeMetadataDoesNotMutateItsInputs(t *testing.T) {
	t.Parallel()

	base := mdOf(t, `{"opts":{"a":1},"own":"x"}`)
	patch := mdOf(t, `{"opts":{"b":2}}`)

	MergeMetadata(base, patch)

	assert.JSONEq(t, `{"opts":{"a":1},"own":"x"}`, string(base.RawMessage))
	assert.JSONEq(t, `{"opts":{"b":2}}`, string(patch.RawMessage),
		"a nested object in the patch must not be shared with the result")
}

func TestMarkerKeysReadsWhatWasRegistered(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when absent", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, MarkerKeys(mapOf(t, `{"own":"x"}`), InheritedKey))
	})

	t.Run("skips non-string entries", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"unit"}, MarkerKeys(mapOf(t, `{"_inherited":[7,"unit",null]}`), InheritedKey))
	})

	t.Run("returns nil for a malformed marker", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, MarkerKeys(mapOf(t, `{"_inherited":"unit"}`), InheritedKey),
			"a damaged marker must not fail the read that carries it")
	})
}

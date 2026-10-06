package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
)

const emptyObject = "{}"

// InheritedKey is the reserved metadata key that lists which of the other
// metadata keys a decorator supplied from an ancestor rather than from the node
// itself. It is a marker, not a property: it is computed on read and is never
// stored, so a write carrying it is rejected instead of merged.
const InheritedKey = "_inherited"

// AggregatedKey is the reserved metadata key that lists which of the other
// metadata keys were computed by aggregating a node's own values with those of
// its descendants. Today it names the projected time span, first and last.
//
// Like InheritedKey it is a marker, computed on read and never stored.
const AggregatedKey = "_aggregated"

// DefaultFirstKey and DefaultLastKey are the metadata keys the TimeSpan
// decorator projects.
//
// They live in model rather than in the decorator package because the
// repository has to strip them from every metadata write, and repository cannot
// import decorator without a cycle: model <- repository <- service <- decorator.
const (
	DefaultFirstKey = "first"
	DefaultLastKey  = "last"
)

// ComputedMarkers lists every marker that decorates a computed value. A write
// carrying any of them is refused rather than merged, so a client learns that
// its echo was discarded instead of silently losing the data.
var ComputedMarkers = []string{InheritedKey, AggregatedKey}

// alwaysComputed lists metadata keys that can only ever be produced by the API.
// Unlike a marker, they are not conditional on the client having echoed one back,
// so they are dropped from every write whether or not a marker names them.
// Without this a client could persist a bare first/last, and a node whose span
// is empty would then serve that fabricated value back as if it were real.
//
// A TimeSpan configured with custom FirstKey/LastKey is not covered here; those
// names are protected only by AggregatedKey, on the echo path.
var alwaysComputed = []string{DefaultFirstKey, DefaultLastKey}

// MarkerKeys returns the metadata keys named by the given marker. A missing,
// malformed or non-string entry is ignored rather than reported: a marker is
// advisory decoration, and a damaged one must not fail the read that carries it.
func MarkerKeys(all map[string]any, marker string) []string {
	raw, ok := all[marker]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(list))
	for _, entry := range list {
		if s, ok := entry.(string); ok {
			keys = append(keys, s)
		}
	}
	return keys
}

// AddMarkerKey records that a decorator computed key into the response under
// marker.
//
// It inserts rather than assigns. Several decorators share one response and run
// in a fixed order, so a plain assignment would let whichever ran last discard
// the other's marker. The list is kept sorted so responses stay comparable, and
// deduplicated because a key may legitimately be computed on more than one pass.
func AddMarkerKey(md map[string]any, marker string, key string) {
	// An empty key would produce a marker naming nothing, which advertises a
	// guarantee the document does not make.
	if md == nil || key == "" {
		return
	}
	keys := MarkerKeys(md, marker)
	for _, existing := range keys {
		if existing == key {
			return
		}
	}
	keys = append(keys, key)
	sort.Strings(keys)

	// Encoded as []any of string so the document matches what decoding it back
	// would yield.
	list := make([]any, len(keys))
	for i, k := range keys {
		list[i] = k
	}
	md[marker] = list
}

// StripComputed returns md reduced to the values actually set on the node.
//
// It drops both markers, every key either marker names, and the keys that can
// only ever be computed by the API. This is the storage guarantee that a
// derived value can never become a stored one. Metadata writes are additive
// ("metadata || $1::jsonb"), so a caller that echoes a decorated node back would
// otherwise persist the marker and the values it names, permanently freezing
// inheritance and span projection for that node. The repository applies this on
// every metadata write, so the guarantee holds even for a path that bypasses a
// decorator.
func StripComputed(md Metadata) Metadata {
	all := md.Map()

	drop := make(map[string]bool, len(alwaysComputed))
	for _, key := range alwaysComputed {
		drop[key] = true
	}
	// The markers are read before the document is reduced, because reading them
	// is what reveals which keys they name.
	for _, marker := range ComputedMarkers {
		for _, key := range MarkerKeys(all, marker) {
			drop[key] = true
		}
		delete(all, marker)
	}
	for key := range drop {
		delete(all, key)
	}

	out, err := MetadataFromMap(all)
	if err != nil {
		// MetadataFromMap only fails on values json cannot encode, and a decoded
		// document always can. Returning the empty object keeps the write
		// happening rather than failing it.
		return Metadata{}
	}
	return out
}

// Metadata is a non-nullable JSON/JSONB document. It always holds a valid JSON
// object, defaulting to an empty object when not set, matching the DB column
// which is NOT NULL DEFAULT '{}'.
type Metadata struct {
	json.RawMessage
}

// NewMetadata returns a Metadata wrapping the given JSON bytes.
func NewMetadata(data []byte) Metadata { return Metadata{RawMessage: data} }

// MetadataFromMap returns a Metadata built from a Go map.
func MetadataFromMap(m map[string]any) (Metadata, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{RawMessage: data}, nil
}

// MergeMetadata returns base with patch applied the way the column's merge
// applies it: a shallow merge of the top-level keys, with nested objects merged
// recursively, and every other value replaced outright. A top-level key set to
// null in the patch is deleted from the result rather than stored as a null,
// which is how a client removes a property. Nested nulls are not interpreted;
// only the top level of the patch is scanned.
//
// It exists so an in-memory mirror of a node's metadata can be updated the same
// way the column is. Metadata writes are additive in SQL, so a service that
// mirrored them by replacement would slowly disagree with the row it shadows: a
// partial patch dropping a key from the mirror would look to every later read
// like an inheritance of that key had been removed.
func MergeMetadata(base, patch Metadata) Metadata {
	values := patch.Map()
	deleted := make([]string, 0, len(values))
	for k, v := range values {
		if v == nil {
			deleted = append(deleted, k)
			delete(values, k)
		}
	}
	merged := mergeMaps(base.Map(), values)
	for _, k := range deleted {
		delete(merged, k)
	}
	out, err := MetadataFromMap(merged)
	if err != nil {
		// Both inputs came from a decoded document, so both are encodable.
		return base
	}
	return out
}

// mergeMaps applies the jsonb || rules for two decoded documents. Neither input
// is modified.
func mergeMaps(base, patch map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(patch))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		patchObj, patchIsObj := v.(map[string]any)
		baseObj, baseIsObj := out[k].(map[string]any)
		// Only two objects merge. Every other combination is a replacement,
		// which is what jsonb does for scalars and arrays alike.
		if patchIsObj && baseIsObj {
			out[k] = mergeMaps(baseObj, patchObj)
			continue
		}
		out[k] = v
	}
	return out
}

// Value implements driver.Valuer so a zero/invalid Metadata still yields {}.
func (m Metadata) Value() (driver.Value, error) {
	if len(m.RawMessage) == 0 || string(m.RawMessage) == "null" {
		return emptyObject, nil
	}
	return string(m.RawMessage), nil
}

// Scan implements sql.Scanner. Accepts []byte, string and nil.
func (m *Metadata) Scan(src any) error {
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case nil:
		m.RawMessage = json.RawMessage(emptyObject)
		return nil
	default:
		return fmt.Errorf("Metadata: cannot scan %T", src)
	}
	if len(data) == 0 || string(data) == "null" {
		data = []byte(emptyObject)
	}
	m.RawMessage = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler, normalizing null to {}.
func (m *Metadata) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		m.RawMessage = json.RawMessage(emptyObject)
		return nil
	}
	m.RawMessage = append(json.RawMessage(nil), data...)
	return nil
}

// MarshalJSON implements json.Marshaler, normalizing an empty or null
// document to {}.
func (m Metadata) MarshalJSON() ([]byte, error) {
	if len(m.RawMessage) == 0 || string(m.RawMessage) == "null" {
		return []byte(emptyObject), nil
	}
	return m.RawMessage, nil
}

// Map returns the metadata as a Go map, defaulting to an empty map. The
// returned map is never nil, so callers may write to it directly. Note that
// unmarshalling the JSON literal null into a map sets it to nil without
// reporting an error, so the result is normalized back to an empty map.
func (m Metadata) Map() map[string]any {
	out := map[string]any{}
	if len(m.RawMessage) == 0 {
		return out
	}
	if err := json.Unmarshal(m.RawMessage, &out); err != nil {
		return map[string]any{}
	}
	if out == nil {
		return map[string]any{}
	}
	return out
}

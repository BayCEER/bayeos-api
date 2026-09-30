package decorator

import "github.com/BayCEER/bayeos/bayeos-api/model"

// View is a mutable facade over a single node, handed to enrichers on their way
// out of the service. The metadata JSON document is unmarshalled once when the
// view is created and marshalled back once on Commit, so a chain of N enrichers
// costs a single JSON round-trip per node rather than one per enricher.
//
// Enrichers must not retain the view beyond the call, and must not call back
// into the service: an enricher runs after the wrapped call has returned, so a
// re-entrant call would recurse.
type View struct {
	Node *model.Node
	md   map[string]any
}

// NewView returns a view over n. A nil node yields an empty view, so enrichers
// do not need a nil check.
func NewView(n *model.Node) *View {
	if n == nil {
		return &View{md: map[string]any{}}
	}
	return &View{Node: n, md: n.Metadata.Map()}
}

// Metadata returns the mutable metadata map. It is never nil, so callers may
// write to it directly. Changes only reach the node on Commit.
func (v *View) Metadata() map[string]any {
	if v.md == nil {
		v.md = map[string]any{}
	}
	return v.md
}

// ID returns the node id, or 0 for an empty view.
func (v *View) ID() int32 {
	if v.Node == nil {
		return 0
	}
	return v.Node.ID
}

// Kind returns the node kind, or the empty string for an empty view.
func (v *View) Kind() string {
	if v.Node == nil {
		return ""
	}
	return v.Node.Kind
}

// DisplayName returns the node display name.
func (v *View) DisplayName() string {
	if v.Node == nil {
		return ""
	}
	return v.Node.DisplayName
}

// Commit writes the metadata map back onto the node. A marshalling failure
// leaves the node untouched rather than clearing its metadata.
func (v *View) Commit() {
	if v.Node == nil {
		return
	}
	merged, err := model.MetadataFromMap(v.md)
	if err != nil {
		return
	}
	v.Node.Metadata = merged
}

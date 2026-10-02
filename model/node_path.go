package model

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidNodePath is wrapped by every path rejection in this file so callers
// can separate a malformed path (the client's fault, 400) from a failed search
// (500) with errors.Is.
var ErrInvalidNodePath = errors.New("invalid node path")

// ValidatePathSegment checks a single path segment, which is also the display
// name of the node it addresses.
//
// The rule mirrors the suppress_wrong_node_display_name trigger in
// migrations/pg/000001_core.up.sql, which rejects a display name containing a
// slash. Keeping the two in agreement matters because a segment accepted here
// but rejected by the trigger would surface as a database error instead of a
// client error, and a segment accepted by the trigger but rejected here would
// make a path the API can produce but not store.
func ValidatePathSegment(seg string) error {
	if seg == "" {
		return fmt.Errorf("%w: segment must not be empty", ErrInvalidNodePath)
	}
	if strings.Contains(seg, "/") {
		return fmt.Errorf("%w: segment %q must not contain %q", ErrInvalidNodePath, seg, "/")
	}
	// "." and ".." are refused even though the trigger would store them, because
	// a search path is matched as a prefix and a segment with traversal
	// semantics in it has no single well-defined target.
	if seg == "." || seg == ".." {
		return fmt.Errorf("%w: segment %q is not a valid path segment", ErrInvalidNodePath, seg)
	}
	if strings.TrimSpace(seg) == "" {
		return fmt.Errorf("%w: segment must not be blank", ErrInvalidNodePath)
	}
	return nil
}

// ValidateNodePath checks an absolute node path, the form the (path).path
// column stores and the API returns in node_path.
//
// Validation is strict: a path is either well formed or rejected. Malformed
// input is not normalized, because the value is compared against stored paths
// rather than used to construct one -- silently rewriting "/root//a" to
// "/root/a" would return hits for a path the client never asked about, and
// would let two different spellings of one path disagree about which node they
// name. Callers that need to build a path from user input should normalize
// first and validate second.
func ValidateNodePath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: path must not be empty", ErrInvalidNodePath)
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: path %q must start with %q", ErrInvalidNodePath, path, "/")
	}
	if path == "/" {
		return nil
	}
	if strings.HasSuffix(path, "/") {
		return fmt.Errorf("%w: path %q must not end with %q", ErrInvalidNodePath, path, "/")
	}

	for _, seg := range strings.Split(path[1:], "/") {
		if err := ValidatePathSegment(seg); err != nil {
			return fmt.Errorf("%w: path %q: %w", ErrInvalidNodePath, path, err)
		}
	}
	return nil
}
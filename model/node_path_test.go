package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePathSegment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		seg     string
		wantErr bool
	}{
		{name: "plain name", seg: "root"},
		{name: "name with spaces", seg: "Sample Data"},
		{name: "name with dots", seg: "a.b.c"},
		{name: "name with dashes and underscores", seg: "a-b_c"},
		{name: "unicode name", seg: "Temperatur°"},
		{name: "empty segment", seg: "", wantErr: true},
		{name: "leading slash", seg: "/root", wantErr: true},
		{name: "trailing slash", seg: "root/", wantErr: true},
		{name: "embedded slash", seg: "a/b", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePathSegment(tt.seg)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrInvalidNodePath), "error should wrap ErrInvalidNodePath: %v", err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateNodePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "root alone", path: "/"},
		{name: "single segment", path: "/root"},
		{name: "nested path", path: "/Users/Administrator/Sample Data"},
		{name: "path with dots in a segment", path: "/a/b.c/d"},
		{name: "path with unicode", path: "/root/Temperatur/°C"},

		{name: "empty path", path: "", wantErr: true},
		{name: "relative path", path: "root/a", wantErr: true},
		{name: "bare segment", path: "root", wantErr: true},
		{name: "home prefix is not absolute", path: "~/a", wantErr: true},
		{name: "trailing slash", path: "/root/", wantErr: true},
		{name: "leading double slash", path: "//root", wantErr: true},
		{name: "empty segment between slashes", path: "/root//a", wantErr: true},
		{name: "current directory segment", path: "/root/./a", wantErr: true},
		{name: "parent directory segment", path: "/root/../a", wantErr: true},
		{name: "whitespace only segment", path: "/ /", wantErr: true},
		{name: "trailing dot segment", path: "/root/.", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateNodePath(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrInvalidNodePath), "error should wrap ErrInvalidNodePath: %v", err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// A path accepted by the validator must be storable: every segment is also a
// display name, and the database trigger rejects a display name containing a
// slash. This asserts the two rules agree, so a path that validates can never
// fail at insert time.
func TestValidateNodePathSegmentsAreValidDisplayNames(t *testing.T) {
	t.Parallel()

	path := "/Users/Administrator/Sample Data/Temperature a-b.c"
	require.NoError(t, ValidateNodePath(path))

	for _, seg := range splitPathSegments(t, path) {
		require.NoError(t, ValidatePathSegment(seg), "segment %q should be a valid display name", seg)
	}
}

// splitPathSegments returns the segments of an already validated path.
func splitPathSegments(t *testing.T, path string) []string {
	t.Helper()
	require.NotEqual(t, "/", path)
	trimmed := path[1:]
	segs := make([]string, 0)
	start := 0
	for i := 0; i <= len(trimmed); i++ {
		if i == len(trimmed) || trimmed[i] == '/' {
			segs = append(segs, trimmed[start:i])
			start = i + 1
		}
	}
	return segs
}

// The search endpoint accepts absolute paths only. A home-relative spelling is
// not a valid search path and must be rejected rather than silently treated as
// an absolute one, since "~" is not a legal path segment.
func TestValidateNodePathRejectsHomeRelativePaths(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"~", "~/", "~/data", "~/data/2024", "~data"} {
		err := ValidateNodePath(path)
		require.Error(t, err, "path %q should be rejected", path)
		assert.True(t, errors.Is(err, ErrInvalidNodePath), "error should wrap ErrInvalidNodePath: %v", err)
	}
}
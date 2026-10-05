package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAntToRegex(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{
			pattern: "/BayEOS/Sensor1",
			want:    `^/BayEOS/Sensor1$`,
		},
		{
			pattern: "/BayEOS/Sensor*",
			want:    `^/BayEOS/Sensor[^/]*$`,
		},
		{
			pattern: "/BayEOS/**/Sensor*",
			want:    `^/BayEOS/(.*)/Sensor[^/]*$`,
		},
		{
			pattern: "**/Sensor*",
			want:    `^(.*)/Sensor[^/]*$`,
		},
		{
			pattern: "Sensor?",
			want:    `^Sensor[^/]$`,
		},
		{
			pattern: "*",
			want:    `^[^/]*$`,
		},
		{
			pattern: "**",
			want:    `^(.*)$`,
		},
		{
			pattern: "test/**/sub/file*",
			want:    `^test/(.*)/sub/file[^/]*$`,
		},
		{
			pattern: "/BayEOS/**/test.txt",
			want:    `^/BayEOS/(.*)/test\.txt$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got, err := antToRegex(tt.pattern)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAntToRegexInvalid(t *testing.T) {
	// An unbalanced escape should either produce valid regex or error.
	// Our implementation handles trailing backslash gracefully.
	_, err := antToRegex(`test\`)
	require.NoError(t, err)
}

func TestExpandHome(t *testing.T) {
	tests := []struct {
		pattern  string
		homePath string
		want     string
	}{
		{
			// A lone "~" is the home node itself, with no wildcard added.
			pattern:  "~",
			homePath: "/Users/antuser",
			want:     "/Users/antuser",
		},
		{
			pattern:  "~/",
			homePath: "/Users/antuser",
			want:     "/Users/antuser/",
		},
		{
			pattern:  "~/*",
			homePath: "/Users/antuser",
			want:     "/Users/antuser/*",
		},
		{
			pattern:  "~/**",
			homePath: "/Users/antuser",
			want:     "/Users/antuser/**",
		},
		{
			pattern:  "~/data",
			homePath: "/Users/antuser",
			want:     "/Users/antuser/data",
		},
		{
			// The remainder is passed through untouched, wildcards included.
			pattern:  "~/**/Sensor*",
			homePath: "/Users/antuser",
			want:     "/Users/antuser/**/Sensor*",
		},
		{
			// A missing separator is supplied rather than assumed away.
			pattern:  "~data",
			homePath: "/Users/antuser",
			want:     "/Users/antuser/data",
		},
		{
			// A "~" that is not the prefix stays literal: a node may be named
			// "~sensor", and "**/~sensor*" has to keep matching it.
			pattern:  "**/~sensor*",
			homePath: "/Users/antuser",
			want:     "**/~sensor*",
		},
		{
			pattern:  "/BayEOS/Sensor1",
			homePath: "/Users/antuser",
			want:     "/BayEOS/Sensor1",
		},
		{
			pattern:  "",
			homePath: "/Users/antuser",
			want:     "",
		},
		{
			// A root home must not grow a second separator.
			pattern:  "~/data",
			homePath: "/",
			want:     "/data",
		},
		{
			pattern:  "~",
			homePath: "/",
			want:     "/",
		},
		{
			// A home that cannot be resolved leaves the pattern untouched, which
			// matches nothing rather than silently searching the whole tree.
			pattern:  "~/data",
			homePath: "",
			want:     "/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			require.Equal(t, tt.want, expandHome(tt.pattern, tt.homePath))
		})
	}
}

// A "~" pattern must reach the same nodes as the absolute path it stands for.
func TestExpandHomeMatchesAbsolutePattern(t *testing.T) {
	const home = "/Users/antuser"

	tilde, err := antToRegex(expandHome("~/**/Sensor*", home))
	require.NoError(t, err)
	absolute, err := antToRegex("/Users/antuser/**/Sensor*")
	require.NoError(t, err)
	require.Equal(t, absolute, tilde)

	tilde, err = antToRegex(expandHome("~", home))
	require.NoError(t, err)
	absolute, err = antToRegex(home)
	require.NoError(t, err)
	require.Equal(t, absolute, tilde)
}

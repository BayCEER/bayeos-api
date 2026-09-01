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

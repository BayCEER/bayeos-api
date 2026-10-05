package service

import (
	"fmt"
	"regexp"
	"strings"
)

// antToRegex converts an Ant-style path pattern into a PostgreSQL-anchored
// regular expression.
//
// Supported wildcards:
//
//	?  → matches exactly one character except /
//	*  → matches zero or more characters except / (within one path segment)
//	** → matches zero or more full path segments (including /)
//
// The returned regex is anchored with ^…$ so it matches the full path.
// When the regex starts with a literal prefix PostgreSQL can still use the
// btree index on (path).path for the prefix portion.
func antToRegex(pattern string) (string, error) {
	// Split on ** first, then convert each part's * and ? wildcards.
	parts := strings.Split(pattern, "**")

	// Convert within-segment wildcards for each part.
	converted := make([]string, 0, len(parts))
	for _, part := range parts {
		var b strings.Builder
		for i := 0; i < len(part); i++ {
			ch := part[i]
			switch ch {
			case '*':
				b.WriteString("[^/]*")
			case '?':
				b.WriteString("[^/]")
			case '\\':
				// escape next char literally
				if i+1 < len(part) {
					i++
					b.WriteString(regexp.QuoteMeta(string(part[i])))
				} else {
					b.WriteString(`\\`)
				}
			case '.', '+', '^', '$', '{', '}', '[', ']', '(', ')', '|':
				b.WriteByte('\\')
				b.WriteByte(ch)
			default:
				b.WriteByte(ch)
			}
		}
		converted = append(converted, b.String())
	}

	// Rejoin with the ** equivalent: (.*)
	regex := "^" + strings.Join(converted, "(.*)") + "$"

	// Validate the generated regex.
	if _, err := regexp.Compile(regex); err != nil {
		return "", fmt.Errorf("invalid ant pattern %q: %w", pattern, err)
	}
	return regex, nil
}

// expandHome rewrites a leading "~" into the caller's home node path, so that
// "~", "~/*" and "~/**" address the nodes their absolute spellings do. It is the
// same prefix middleware.ResolvePathOrIdFromBody already accepts in the node_path
// of a create request; doing it here rather than in the handler keeps the path
// syntax in one place, so create and search cannot drift apart.
//
// Only the first segment is replaced and no wildcard is added: "~" addresses the
// home node itself, and every other spelling brings its own. A pattern whose "~"
// is not the first character is left alone, so "**/~sensor*" still matches a node
// literally named "~sensor".
func expandHome(pattern string, homePath string) string {
	if pattern == "" || pattern[0] != '~' {
		return pattern
	}

	// A home node is an ordinary node, but trimming keeps a root home from
	// producing "//data".
	home := strings.TrimSuffix(homePath, "/")
	rest := pattern[1:]

	if home == "" {
		if rest == "" {
			return "/"
		}
		return "/" + strings.TrimPrefix(rest, "/")
	}
	if rest == "" {
		return home
	}
	// The separator is only added when the caller left it out, so "~/*" does not
	// become "//*".
	if strings.HasPrefix(rest, "/") {
		return home + rest
	}
	return home + "/" + rest
}

package decorator

// Selector decides whether a decorator applies to an individual node. A nil
// Selector is treated as "applies to every node", so decorators that are not
// selective do not need to set one.
type Selector func(v *View) bool

// Always returns a Selector that matches every node.
func Always() Selector {
	return func(*View) bool { return true }
}

// Never returns a Selector that matches no node, which disables a decorator
// while leaving it in the composition list.
func Never() Selector {
	return func(*View) bool { return false }
}

// KindIs returns a Selector matching nodes whose Kind is one of kinds. The set
// is built once at construction rather than scanned per node, because
// selectors are evaluated for every node of every read.
//
// An empty or nil kinds list matches every node, so a decorator with an
// unconfigured kind filter behaves as if it had no filter at all.
func KindIs(kinds ...string) Selector {
	if len(kinds) == 0 {
		return Always()
	}
	set := make(map[string]struct{}, len(kinds))
	for _, k := range kinds {
		set[k] = struct{}{}
	}
	return func(v *View) bool {
		_, ok := set[v.Kind()]
		return ok
	}
}

// All returns a Selector matching only when every selector in s matches. With
// no arguments it matches every node.
func All(s ...Selector) Selector {
	if len(s) == 0 {
		return Always()
	}
	return func(v *View) bool {
		for _, sel := range s {
			if sel != nil && !sel(v) {
				return false
			}
		}
		return true
	}
}

// Any returns a Selector matching when at least one selector in s matches. With
// no arguments it matches every node.
func Any(s ...Selector) Selector {
	if len(s) == 0 {
		return Always()
	}
	return func(v *View) bool {
		for _, sel := range s {
			if sel != nil && sel(v) {
				return true
			}
		}
		return false
	}
}

// Not inverts a selector. A nil inner selector yields Never.
func Not(s Selector) Selector {
	if s == nil {
		return Never()
	}
	return func(v *View) bool { return !s(v) }
}

// matches reports whether sel applies to v, treating nil as always.
func matches(sel Selector, v *View) bool {
	return sel == nil || sel(v)
}

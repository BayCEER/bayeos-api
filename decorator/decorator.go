// Package decorator provides a code-configured chain of loosely coupled
// decorators around service.NodeService.
//
// The service itself stays free of cross-cutting knowledge: it neither knows
// which decorators are applied nor how they are ordered. Cross-cutting
// behaviour is declared as a static slice in a single composition function
// (see decorateNodes in package main), so adding a capability is one line
// rather than an edit to the service.
//
// # Ordering
//
// Two independent orderings, both fixed by declaration order:
//
//   - Wrap is applied outside-in. The first Decorator in the slice is the
//     outermost layer and sees a call first on the way in and last on the way
//     out.
//   - Enrich runs after the wrapped call returns, in declaration order, on the
//     nodes the call produced. Where two enrichers write the same metadata
//     key, the later one wins.
//
// # Failure policy
//
// Wrap panics are recovered by the HTTP error middleware at the request
// boundary. Enrich is fail-open: a panicking enricher is recovered and logged,
// and the node is still returned, because decoration is additive and the node
// data the service produced is already correct.
//
// # Dependencies
//
// This package imports service, never the reverse, so service.NodeService can
// be decorated without an import cycle. Decorators are tested against a stub
// service that implements only the methods the decorator touches.
package decorator

import (
	"context"

	"github.com/BayCEER/bayeos/bayeos-api/service"
)

// Decorator is one composable unit of cross-cutting node behaviour. Both
// fields are optional: Wrap layers behaviour onto the service, Enrich mutates
// nodes on their way out. A decorator that needs only one of them leaves the
// other nil.
type Decorator struct {
	// Name identifies the decorator in logs and in the boot-time chain dump.
	Name string

	// When decides which nodes an Enrich function applies to. A nil selector
	// applies to every node. It is not consulted by Wrap.
	When Selector

	// Wrap returns a service.NodeService that delegates to next. A nil Wrap
	// leaves the chain unchanged at this position.
	Wrap func(next service.NodeService) service.NodeService

	// Enrich mutates a single node after the wrapped call returned. A nil
	// Enrich contributes nothing.
	Enrich func(ctx context.Context, v *View)
}

// Chain composes base with decs and returns the outermost service.NodeService.
// Chain always returns a non-nil service, so callers can use it directly as a
// dependency.
func Chain(base service.NodeService, decs ...Decorator) service.NodeService {
	svc := base
	for i := len(decs) - 1; i >= 0; i-- {
		if decs[i].Wrap != nil {
			svc = decs[i].Wrap(svc)
		}
	}
	if e := activeEnrichers(decs); len(e) > 0 {
		svc = &enriched{NodeService: svc, enrichers: e}
	}
	return svc
}

// Names returns the names of decs in declaration order, for logging.
func Names(decs []Decorator) []string {
	names := make([]string, len(decs))
	for i, d := range decs {
		names[i] = d.Name
	}
	return names
}

// activeEnrichers collects the enrich functions of decs, dropping the ones with
// no Enrich. Selectors are kept alongside so they are evaluated per node.
func activeEnrichers(decs []Decorator) []enricher {
	var out []enricher
	for _, d := range decs {
		if d.Enrich == nil {
			continue
		}
		out = append(out, enricher{when: d.When, fn: d.Enrich})
	}
	return out
}

// enricher pairs an enrich function with its selector.
type enricher struct {
	when Selector
	fn   func(ctx context.Context, v *View)
}

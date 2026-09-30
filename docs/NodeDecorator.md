# The Node Decorator Pattern

`decorator/` provides a code-configured chain of loosely coupled decorators around
`service.NodeService`. This document explains the pattern in general, then how it
is implemented in this repository.

The time span tree that the `TimeSpan` decorator depends on is documented
separately in [NodeTimeSpanService.md](NodeTimeSpanService.md).

---

## Part 1 — The pattern

### The core move

Wrap a target object in another object **with the same interface** that delegates
every call and adds behaviour on the way through.

```go
type LoggingNodeService struct {
	service.NodeService // embedded: anything not overridden passes straight through
}

func (l *LoggingNodeService) GetNode(ctx context.Context, id, userID int32) (model.Node, error) {
	start := time.Now()
	n, err := l.NodeService.GetNode(ctx, id, userID) // delegate to the wrapped service
	log.Debug().Dur("took", time.Since(start)).Msg("GetNode")
	return n, err // same value, same error
}
```

Three properties make this worth the indirection.

**1. The wrapper is substitutable.** Because `*LoggingNodeService` satisfies
`service.NodeService`, no caller can tell the difference. A handler holding a
`NodeService` needs no changes, and the wrapper can be added, removed or reordered
without touching either the service or its consumers.

**2. Composition replaces inheritance.** The alternative is a type per combination:
`LoggingTimeSpanNodeService`, `TimeSpanLoggingNodeService`, and so on,
multiplying with every new behaviour. Wrapping multiplies nothing — behaviours are
just nested, and a combination that nobody configures costs nothing.

**3. The target stays ignorant.** This is the reason the pattern is used here.
`NodeService` does not know that it is timed, logged, or that its payload is about
to gain `first` and `last` metadata. Without decoration, every cross-cutting
concern becomes a permanent feature of the service — and a permanent cost for the
deployments that do not want it.

### Why it is here, specifically

`NodeService` has a read surface of five methods that all return nodes
(`GetNode`, `GetNodes`, `GetNodeChildren`, `GetNodeParents`,
`FindNodesByAntPath`). Two kinds of cross-cutting concern apply to it:

- **Around the call** — request logging, timing, caching, transactions, and
  keeping an in-memory projection in step with writes.
- **On the result** — projecting derived data into each node's metadata, such as
  the aggregated time span a folder covers.

Both are properties of _how the service is deployed_, not of what a node is.
Baking either into the service would make it impossible to turn off.

---

## Part 2 — The implementation here

### The two hooks

This implementation splits decoration into two orthogonal halves. This split is
the main departure from the textbook pattern and the most important thing to
understand before reading the code.

| Hook                                                      | Runs                                   | Sees                           | Use for                                                  |
| --------------------------------------------------------- | -------------------------------------- | ------------------------------ | -------------------------------------------------------- |
| `Wrap func(next service.NodeService) service.NodeService` | _around_ the call, before and after    | arguments, results, **errors** | logging, timing, caching, transactions, write-sync, auth |
| `Enrich func(ctx context.Context, v *View)`               | _after_ the call returns, on each node | the result only                | projecting derived data into the payload                 |

`Decorator` (`decorator.go:45-60`) carries both, plus a `Name` for logging and a
`When` selector. Both hooks are optional; a decorator that needs only one leaves
the other nil.

The distinction matters because the two have **different failure semantics**:

- `Wrap` may change control flow, so a failure there is a genuine request failure.
  A panic in `Wrap` propagates and is recovered by the HTTP error middleware at
  the request boundary.
- `Enrich` is pure post-processing of a result that is already correct. Decoration
  is additive, so a broken enricher must not fail the request. `node_enrich.go:107-116`
  recovers the panic, logs it with the node id, and still returns the node.

```go
func (e *enriched) apply(ctx context.Context, en enricher, v *View) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Log().Error().
				Interface("panic", rec).
				Int("node.id", int(v.ID())).
				Msg("node decorator panicked; node left undecorated")
		}
	}()
	en.fn(ctx, v)
}
```

### The read surface is implemented once

A naive reading of the pattern has every decorator override all five
node-returning methods, because that is what it takes to post-process their
results. Adding a decorator would then mean five new methods.

Here, `enriched` (`node_enrich.go:18-71`) implements those five methods **once**
and holds a list of enrichers:

```go
type enriched struct {
	service.NodeService
	enrichers []enricher
}

func (e *enriched) GetNode(ctx context.Context, id, userID int32) (model.Node, error) {
	n, err := e.NodeService.GetNode(ctx, id, userID)
	if err != nil {
		return n, err
	}
	e.run(ctx, &n)
	return n, nil
}
```

All other methods are promoted from the embedded `NodeService` unchanged. A new
decorator therefore contributes one `Enrich` function and touches no service
method at all. The intent is stated at `node_enrich.go:12-15`: a change to the read
surface of `NodeService` should surface in one place, not in every decorator.

`run` (`node_enrich.go:85-102`) evaluates each enricher's selector per node and
commits only if at least one enricher actually ran, so a chain that selects
nothing leaves the payload byte-for-byte identical.

### Building the chain

`Chain` (`decorator.go:65-76`) composes in two steps:

```go
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
```

The loop runs **backwards**, so the first declared `Wrap` ends up outermost. The
`enriched` layer is installed **last**, making it the outermost layer overall —
enrichment therefore happens after the whole `Wrap` chain has returned.

`Chain` always returns a non-nil service, so callers can use it directly as a
dependency. `Names` (`decorator.go:79-85`) returns the declared names in order for
the boot-time log line.

### The two orderings

Both are fixed by declaration order, and they are independent of each other.

**`Wrap` is applied outside-in.** The first entry in the slice is the outermost
layer: it sees a call first on the way in and last on the way out.

**`Enrich` runs after the wrapped call returns, in declaration order.** Where two
enrichers write the same metadata key, the **later one wins**.

These interact. Enrichers always run outside all `Wrap` layers, so `Wrap` behaviour
cannot observe or override an enriched payload.

### `View` — why a facade and not the raw node

`View` (`node_view.go:13-16`) is a mutable facade over a single node:

```go
type View struct {
	Node *model.Node
	md   map[string]any
}
```

The reason for the facade is cost. The metadata is a JSON document; it is
unmarshalled once in `NewView` and marshalled back once in `Commit`
(`node_view.go:20-25`, `62-70`). A chain of N enrichers therefore costs **one JSON
round-trip per node**, not one per enricher. Without it, every enricher would
re-parse and re-serialise the same document.

`Commit` is a no-op on a marshalling failure rather than clearing the metadata, and
`NewView` on a nil node yields an empty view so enrichers need no nil check.

Two rules are documented at `node_view.go:10-12`:

- **Do not retain the view** beyond the call.
- **Do not call back into the service** from an enricher. Enrich runs after the
  wrapped call has already returned, so a re-entrant call would recurse.

### `Selector` — which nodes a decorator applies to

`Selector` (`node_selector.go:6`) is a predicate over a `View`. A `nil` selector
means "applies to every node", so a decorator that is not selective does not have
to set one (`matches`, `node_selector.go:80-82`).

| Constructor        | Matches                                                        |
| ------------------ | -------------------------------------------------------------- |
| `Always()`         | every node                                                     |
| `Never()`          | no node — disables a decorator while leaving it in the list    |
| `KindIs(kinds...)` | nodes whose `Kind` is one of `kinds`; empty matches every node |
| `All(s...)`        | every selector in `s`; no arguments matches every node         |
| `Any(s...)`        | at least one selector in `s`; no arguments matches every node  |
| `Not(s)`           | inverts `s`; a nil inner selector yields `Never`               |

`KindIs` builds its set once at construction rather than scanning `kinds` per node,
because selectors are evaluated for **every node of every read**.

The empty-`kinds` case is worth knowing: an unconfigured kind filter matches
everything rather than nothing. For `TimeSpan` that is the wrong default, because
only a kind that actually carries measurements has a time span.

### The composition root

Cross-cutting behaviour is declared in exactly one place — `decorateNodes` in
`nodes.go:36-50`. It is a static slice evaluated once at start-up: no registry, no
discovery, no runtime reconfiguration.

```go
func decorateNodes(base service.NodeService, span service.NodeTimeSpanServiceInterface, spanLoader func(context.Context) ([]model.NodeTimeSpan, error), props service.NodePropertyServiceInterface, propLoader func(context.Context) ([]model.Node, error)) service.NodeService {
	chain := []decorator.Decorator{
		decorator.TimeSpan(decorator.TimeSpanConfig{
			Span:          span,
			Loader:        spanLoader,
			Kinds:         timeSpanKinds,
			OmitWhenEmpty: true,
			SyncWrites:    true,
			RefreshEvery:  timeSpanRefreshInterval,
		}),
		decorator.InheritedProps(decorator.InheritedConfig{
			Props:        props,
			Rules:        propertyRules,
			Loader:       propLoader,
			SyncWrites:   true,
			RefreshEvery: timeSpanRefreshInterval,
		}),
	}

	log.Infof("active node decorators: %v", decorator.Names(chain))
	return decorator.Chain(base, chain...)
}
```

Adding a capability is one line here; removing the line removes the behaviour. The
active chain is also logged at start-up, so what is installed is visible without
reading the code.

### Worked example — adding a decorator

To add, say, a thumbnail URL to the metadata of every timeseries node, add one
file and one line.

```go
// decorator/node_thumbnail.go
package decorator

func Thumbnail(baseURL string) Decorator {
	return Decorator{
		Name: "thumbnail",
		When: KindIs(model.KindTimeseries),
		Enrich: func(_ context.Context, v *View) {
			md := v.Metadata()
			md["thumbnail"] = baseURL + "/thumb/" + strconv.Itoa(int(v.ID()))
		},
	}
}
```

```go
// nodes.go
chain := []decorator.Decorator{
	decorator.TimeSpan(/* ... */),
	decorator.InheritedProps(/* ... */),
	decorator.Thumbnail(cfg.ThumbnailBaseURL),
}
```

That is the whole change. No service method is touched, no other decorator is
affected, and because `When` selects on kind, folders are left alone. Note that
adding the entry **last** means the thumbnail enricher sees metadata that
`TimeSpan` and `InheritedProps` have already written.

If the new capability needed to see the request and its errors, it would set
`Wrap` instead and would appear in the ordering discussion above.

### The InheritedProps decorator

`InheritedProps` (`node_inherited.go`) is the second installed decorator. It
inherits a node's metadata down the tree, so a unit set once on a folder applies
to every series beneath it.

**What a rule names.** A `service.PropertyRule` names a source *kind*, never a
property: every key that kind of node holds is inherited. `propertyRules` in
`nodes.go` is therefore a single `{SourceKind: model.KindFolder}` rule, and a
property nobody anticipated on a folder still reaches its descendants. The
trade-off is deliberate — the set of inheritable properties cannot be narrowed
to a known list, and a folder's private bookkeeping keys are inherited too.

**Where the values go.** Inherited properties are merged into `metadata`, not
exposed as a separate field or a separate document. That keeps every existing
reader of `unit`, `lat`, `lon` and `resolution` working unchanged — including
`resolveGapThreshold` in the timeseries service, which reads the resolution off
the decorated node it already fetches.

**How they are marked.** A node's own metadata says nothing about provenance, so
the decorator writes the list of keys it actually injected into
`model.InheritedKey` (`_inherited`), as a sorted array of strings. The marker is
set only when something was inherited, so a node with nothing to inherit is
returned byte-for-byte unchanged. A property the node already defines is *not*
listed: the value returned is the node's own, and the node is free to change it,
so the marker must not claim otherwise.

### The two computed markers

Two decorators write provenance into the same metadata document, and they own
different markers:

| Marker | Written by | Names |
| --- | --- | --- |
| `_inherited` | `InheritedProps` | keys copied in from an ancestor on this read |
| `_aggregated` | `TimeSpan` | `first`, `last`, rolled up from this node and its descendants |

`model.AddMarkerKey` inserts into the list rather than assigning it. The
decorators share one response and run in a fixed order, so a plain assignment
would let whichever ran last discard the other's marker. Entries are sorted so
responses stay comparable, and deduplicated because a key may legitimately be
computed on more than one pass.

**Resolution.** `NodePropertyService.Inherited`
(`service/node_property_service.go`) walks the parent chain on every read and
takes the nearest value it finds for each key, so the nearest ancestor wins for
the keys it holds while the rest are still collected further up. The walk
therefore always reaches the root, bounded by a cycle guard rather than by a
"found enough" test. The tree holds only locally stored values, so a change to an
ancestor takes effect on the next read with no rebuild, and a derived value can
never drift into being local.

**The write invariant.** `MergeNodeMetadata` (`repository/node_repo.go:170`) runs
`metadata = metadata || $1::jsonb`, which is additive. Without a guard, a client
that GETs a node and PATCHes the body back would store the inherited value
permanently, silently freezing inheritance for that node.

The guarantee is enforced in two layers, because the storage layer is the only
one that cannot be bypassed:

- **The repository strips it.** `model.StripComputed` is applied on every
  metadata write — `CreateNode`, `UpdateNode` and `MergeNodeMetadata`. It removes
  every computed marker *and* every key a marker names, so the column can only
  ever hold values that were set on the node itself. Dropping the keys matters as
  much as dropping the marker: keeping the value while removing the marker would
  leave a derived value stored as if it were local, which is the corruption being
  prevented.
- **The decorator rejects it.** `rejectComputedMetadata` refuses any write
  carrying `_inherited` or `_aggregated`, with
  `repository.ErrInheritedNotWritable` or `repository.ErrAggregatedNotWritable`
  matching the marker actually present. The handlers map both to 400
  (`AbortWithBadRequestIfComputedNotWritable`). This covers create,
  create-by-path, update and metadata-update, on both decorators.

The marker guard is installed on both decorators whether or not `SyncWrites` is
on. Refusing an echoed read is a contract with the client, not bookkeeping: with
the guard tied to `SyncWrites`, switching write-sync off would silently start
persisting markers and fabricated span values. The tree bookkeeping each wrapper
owns is what `SyncWrites` gates, and each of those calls no-ops when it is off.

Rejecting at the edge beats stopping at the repository. Stripping alone would
remove the keys the marker names and leave the client with a silent no-op: a user
who edited an inherited value and submitted the object would have the edit
discarded with no diagnostic. The 400 names the offending key, and because the
markers are new, no existing client sends them. The supported way to override an
inherited value is to send the property without the marker, which writes it as an
ordinary local value.

**`first` and `last` are dropped even without a marker.** `StripComputed` also
removes `model.DefaultFirstKey` and `model.DefaultLastKey` unconditionally. A
client that hand-writes them without echoing a marker would otherwise be able to
pin a fabricated span to a node and have it served back as though it were
measured. A `TimeSpan` configured with custom `FirstKey`/`LastKey` is not covered
by that unconditional list — those names are protected by `_aggregated` alone, on
the echo path.

The write half also keeps the tree current — `MoveNode` on a `parent_id` change,
`SetLocalMetadata` on a metadata write, `AddNode` on create, and a single
`DeleteNodes` batch on delete. As with `TimeSpan`, `CreateNodeByPath` is not
tracked because its intermediate folder ids are never returned, and
`RefreshEvery` is the safety net for both that and the folder created directly by
sign-up.

**A stale tree fails silently.** The tree is keyed by node id and nothing on the
read path can tell a missing node from a node with no inheritable metadata:
`Inherited` returns `nil` in both cases, and `SetLocalMetadata` ignores an unknown
node. So a tree built before the database was restored or recreated maps an id
onto the *wrong* ancestry and quietly resolves nothing — the response just has no
`_inherited` key, with no error and no log. Postgres restarts its id sequence
after a drop and recreate, which makes this easy to hit: a server left running
across a reset keeps answering from the pre-reset tree.

`refreshInterval` (`nodes.go:19-31`) is set to one minute to bound that window,
down from ten. It is shared with the time span tree, so this is one node query
per minute per tree; raise it if the node table grows enough for that to matter.
A restart builds the tree from current rows immediately, which is still the
fastest way to recover.

### The TimeSpan decorator as a case study

`TimeSpan` (`node_timespan.go:66-99`) uses both halves, which makes it the
clearest example of the split.

**Read half.** `timeSpanEnricher` (`node_timespan.go:102-116`) projects the
aggregated span of a node into its metadata as RFC3339 strings. A zero bound
renders as the empty string rather than a misleading year-one timestamp
(`formatTime`, `node_timespan.go:120-125`). `OmitWhenEmpty` skips nodes with no
span at all rather than writing zeros.

**Write half.** `timeSpanService` (`node_timespan.go:129-207`) embeds the wrapped
`NodeService` and overrides only the mutating methods, keeping the in-memory span
tree in step so projected values do not go stale between refreshes. Every method it
does not override is promoted unchanged.

Two write paths are deliberately **not** tracked, and the comments say why:

- `CreateNodeByPath` — the intermediate folders created along the path are not
  returned by the service, so their ids are unknown.
- `UpdateNodeMetadata` — metadata holds no measurements.

Neither can be observed from the decorator, which is why `RefreshEvery` exists.
The refresher (`startTimeSpanRefresher`, `node_timespan.go:263-279`) rebuilds the
tree on an interval into a swappable holder (`syncSpan`,
`node_timespan.go:228-240`). A failure is logged and the previous tree is kept, so
a transient database problem degrades freshness rather than losing the decorator.
The holder is installed **before** the enricher and wrapper are built
(`node_timespan.go:79-86`), so both capture the holder rather than the initial
tree, and an in-flight request never observes a half-swapped tree.

### Dependency direction

`decorator` imports `service`, never the reverse. This is what allows
`service.NodeService` to be decorated at all — the other direction would be an
import cycle. Cross-cutting code depends on the service, not the other way round,
and `NodeService` therefore contains no knowledge of which decorators are applied
or how they are ordered.

### Testing

Decorators are tested against a stub service (`node_stub_test.go`) that **embeds**
`service.NodeService` and implements only the methods a given test exercises. A
test that only covers `DeleteNodesByIDs` implements that one method; anything else
would panic on a nil embedded interface, which is itself a useful signal.

The chain is tested through the public surface: `Chain(base, TimeSpan(cfg))` and
then ordinary service calls, asserting on what comes back.

`InheritedProps` is additionally covered at the HTTP boundary in
`handler/node_inherited_handler_test.go`, which is where the round-trip hazard
actually lives. The property service is tested on its own in
`service/node_property_service_test.go` for precedence, cycles, deletes and
marker handling.

Note that the handler test suite builds its own chain in
`newTestNodeService` (`handler_test.go`), because the test binary cannot import
package `main`. That helper must be kept in step with `decorateNodes`. The time
span decorator is left out there because it needs a QuestDB connection the
handler tests do not provide.

### Trade-offs

Decorators are not free, and these are the costs as they apply here.

**Behaviour becomes invisible at the call site.** A handler that receives a
decorated service cannot see what has been applied to it. The mitigations here are
the single visible composition slice in `nodes.go` and the start-up log line that
prints the active chain. A reader who only looks at a handler will not know a
decorator is in play.

**Order is load-bearing.** Because both orderings come from declaration order,
reordering the slice is a behaviour change. `Enrich` collisions resolve last-wins
with no warning, so two enrichers writing the same key is a silent conflict rather
than a compile error.

**A failing enricher degrades silently.** That is deliberate — the alternative is
failing requests over optional presentation data — but it does mean a broken
decorator shows up as a missing field rather than a failed request. The
fail-open log is the only signal.

**Extra indirection on every call.** Each `Wrap` layer is one more interface hop.
This is cheap relative to the database round-trip a node read already performs,
but it is not zero, and it is why a decorator that only enriches metadata should
use `Enrich` and leave `Wrap` nil.

**When not to use this.** If the behaviour is intrinsic to what a node _is_
rather than to how it is served, it belongs in the service or the model. If it
must apply to every caller unconditionally, a decorator adds a layer without
adding a choice. And if only one implementation will ever be configured, the
pattern is a lot of machinery for a single wrapper.

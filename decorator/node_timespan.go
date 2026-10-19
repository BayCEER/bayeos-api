package decorator

import (
	"context"
	"sync"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"

	log "github.com/BayCEER/bayeos/bayeos-api/logger"
)

// Default metadata keys written by the TimeSpan decorator. They are aliased from
// model because the repository strips them from every metadata write and cannot
// import this package.
const (
	DefaultFirstKey = model.DefaultFirstKey
	DefaultLastKey  = model.DefaultLastKey
)

// TimeSpanConfig configures the TimeSpan decorator.
//
// The decorator has two halves. On the read path it projects the aggregated
// time span of a node into its metadata. On the write path it keeps the
// in-memory span tree in step with node mutations, so the projected values do
// not go stale between periodic refreshes.
type TimeSpanConfig struct {
	// Span is the aggregated time span tree. Required.
	Span service.NodeTimeSpanServiceInterface

	// Loader rebuilds the tree from scratch. It is required when RefreshEvery
	// is set, and is also the source used for the initial load.
	Loader func(ctx context.Context) ([]model.NodeTimeSpan, error)

	// Kinds restricts the decorator to nodes of these kinds. An empty or nil
	// list applies it to every node, which is usually wrong: only a kind that
	// actually carries measurements has a time span. Folders receive the
	// rolled-up span of their subtree.
	Kinds []string

	// FirstKey and LastKey are the metadata keys written. Empty means
	// DefaultFirstKey and DefaultLastKey.
	FirstKey string
	LastKey  string

	// OmitWhenEmpty skips nodes whose span is unknown or whose first and last
	// are both zero, rather than writing zero timestamps.
	OmitWhenEmpty bool

	// SyncWrites maintains the tree as nodes are created, moved and deleted.
	// Leave it off when the span is only recomputed on a schedule.
	SyncWrites bool

	// RefreshEvery periodically rebuilds the tree via Loader. It is the safety
	// net for mutations this decorator cannot observe, in particular the
	// Folder created directly through the repository by user sign-up, and for
	// path based creation where the intermediate node ids are never returned.
	// Zero disables the refresher.
	RefreshEvery time.Duration
}

// TimeSpan returns a Decorator that projects a node's aggregated time span into
// its metadata as RFC3339 strings.
//
// The returned Decorator sets both Wrap and Enrich, so it occupies one entry in
// the composition list regardless of which halves are enabled.
func TimeSpan(cfg TimeSpanConfig) Decorator {
	if cfg.Span == nil {
		panic("decorator: TimeSpan requires a Span")
	}
	if cfg.FirstKey == "" {
		cfg.FirstKey = DefaultFirstKey
	}
	if cfg.LastKey == "" {
		cfg.LastKey = DefaultLastKey
	}

	// The refresher is installed before the enricher and wrapper are built, so
	// both capture the swappable holder rather than the initial tree.
	if cfg.RefreshEvery > 0 {
		if cfg.Loader == nil {
			panic("decorator: TimeSpan RefreshEvery requires a Loader")
		}
		holder := &syncSpan{impl: cfg.Span}
		cfg.Span = holder
		startTimeSpanRefresher(holder, cfg.Loader, cfg.RefreshEvery)
	}

	d := Decorator{
		Name:   "timespan",
		When:   KindIs(cfg.Kinds...),
		Enrich: timeSpanEnricher(cfg),
		// The wrapper is installed whether or not writes are being synced.
		// Refusing an echoed read is a contract with the client, not bookkeeping:
		// leaving it tied to SyncWrites would silently start persisting markers
		// and fabricated span values the moment write-sync was switched off.
		// The methods it owns no-op on their span bookkeeping when SyncWrites is
		// false, so the guard is the only behaviour that remains.
		Wrap: func(next service.NodeService) service.NodeService {
			return &timeSpanService{NodeService: next, cfg: cfg}
		},
	}
	return d
}

// timeSpanEnricher projects the aggregated span into node metadata.
func timeSpanEnricher(cfg TimeSpanConfig) func(context.Context, *View) {
	return func(_ context.Context, v *View) {
		first, last, ok := cfg.Span.GetTimeSpan(v.ID())
		if !ok {
			return
		}
		if cfg.OmitWhenEmpty && first.IsZero() && last.IsZero() {
			return
		}

		md := v.Metadata()
		md[cfg.FirstKey] = formatTime(first)
		md[cfg.LastKey] = formatTime(last)
		// Register the two keys as computed. They are rolled up from this node
		// and its descendants, so the marker tells a client they are not the
		// node's own values, and names them so the repository can strip them
		// from a write. Added rather than assigned: another decorator shares
		// this metadata and must not have its marker discarded here.
		model.AddMarkerKey(md, model.AggregatedKey, cfg.FirstKey)
		model.AddMarkerKey(md, model.AggregatedKey, cfg.LastKey)
	}
}

// formatTime renders a span bound as UTC RFC3339. A zero time renders as the
// empty string rather than as a misleading year-one timestamp.
func formatTime(t time.Time) any {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// timeSpanService is the write-path half of the TimeSpan decorator. Every
// method it does not override is promoted from the wrapped service unchanged.
type timeSpanService struct {
	service.NodeService
	cfg TimeSpanConfig
}

var _ service.NodeService = (*timeSpanService)(nil)

// CreateNode registers the new node in the span tree. A new node holds no
// measurements yet, so the span service needs no re-aggregation here. A computed
// marker in the body is still refused: this decorator is what produces them, so
// it is the right place to catch an echoed read.
func (s *timeSpanService) CreateNode(ctx context.Context, node *model.Node, access model.NodeAccess) error {
	if node != nil {
		if err := rejectComputedMetadata(node.Metadata.Map()); err != nil {
			return err
		}
	}
	if err := s.NodeService.CreateNode(ctx, node, access); err != nil {
		return err
	}
	if node == nil {
		return nil
	}

	if s.cfg.SyncWrites {
		s.cfg.Span.AddNode(node.ID, node.ParentID)
	}
	return nil
}

// CreateNodeByPath is not tracked incrementally: the intermediate folders
// created along the path are not returned by the service, so their ids are
// unknown. The next refresh picks them up.
func (s *timeSpanService) CreateNodeByPath(ctx context.Context, node *model.Node, baseNodeID int32, path string, access model.NodeAccess) error {
	if node != nil {
		if err := rejectComputedMetadata(node.Metadata.Map()); err != nil {
			return err
		}
	}
	return s.NodeService.CreateNodeByPath(ctx, node, baseNodeID, path, access)
}

// UpdateNode treats a parent_id change as a move, which changes which ancestors
// a node's data rolls up into.
func (s *timeSpanService) UpdateNode(ctx context.Context, id int32, fields *map[string]any) error {
	if err := rejectComputedFields(fields); err != nil {
		return err
	}
	if err := s.NodeService.UpdateNode(ctx, id, fields); err != nil {
		return err
	}

	var newParent int32
	moved := false
	if fields != nil {
		if raw, ok := (*fields)["parent_id"]; ok {
			if p, ok := toInt32(raw); ok {
				newParent = p
				moved = true
			}
		}
	}
	if !moved {
		return nil
	}

	if s.cfg.SyncWrites {
		s.cfg.Span.MoveNode(id, newParent)
	}
	return nil
}

// UpdateNodeMetadata needs no span bookkeeping: metadata holds no measurements.
// The computed-marker guard still applies, since this is the shortest path a
// client has to write back a node it just read.
func (s *timeSpanService) UpdateNodeMetadata(ctx context.Context, id int32, metadata map[string]any) error {
	if err := rejectComputedMetadata(metadata); err != nil {
		return err
	}
	return s.NodeService.UpdateNodeMetadata(ctx, id, metadata)
}

// DeleteNodesByIDs is the method the node handler actually calls to delete; the
// singular DeleteNode is not on the delete path. The span service cascades over
// descendants and re-aggregates internally, so passing the whole id set costs
// one rebuild rather than one per id.
func (s *timeSpanService) DeleteNodesByIDs(ctx context.Context, ids []int32) error {
	if err := s.NodeService.DeleteNodesByIDs(ctx, ids); err != nil {
		return err
	}

	if s.cfg.SyncWrites {
		s.cfg.Span.DeleteNodes(ids)
	}
	return nil
}

// DeleteNode implements the singular delete for completeness.
func (s *timeSpanService) DeleteNode(ctx context.Context, id int32) error {
	if err := s.NodeService.DeleteNode(ctx, id); err != nil {
		return err
	}

	if s.cfg.SyncWrites {
		s.cfg.Span.DeleteNode(id)
	}
	return nil
}

// toInt32 normalises the numeric types a JSON body decodes parent_id to.
func toInt32(v any) (int32, bool) {
	switch n := v.(type) {
	case int32:
		return n, true
	case int:
		return int32(n), true
	case int64:
		return int32(n), true
	case float64:
		return int32(n), true
	case float32:
		return int32(n), true
	default:
		return 0, false
	}
}

// syncSpan is a swappable holder around a span tree, so a periodic rebuild can
// replace the tree without racing the in-flight requests that read it.
type syncSpan struct {
	mu   sync.RWMutex
	impl service.NodeTimeSpanServiceInterface
}

func (s *syncSpan) current() service.NodeTimeSpanServiceInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.impl
}

func (s *syncSpan) set(impl service.NodeTimeSpanServiceInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.impl = impl
}

func (s *syncSpan) SetLocalMetrics(nodeID int32, first, last time.Time) {
	s.current().SetLocalMetrics(nodeID, first, last)
}
func (s *syncSpan) AddNode(nodeID int32, parentID int32) { s.current().AddNode(nodeID, parentID) }
func (s *syncSpan) AddNodes(nodes []model.NodeTimeSpan)  { s.current().AddNodes(nodes) }
func (s *syncSpan) MoveNode(nodeID int32, newParentID int32) {
	s.current().MoveNode(nodeID, newParentID)
}
func (s *syncSpan) DeleteNode(nodeID int32)     { s.current().DeleteNode(nodeID) }
func (s *syncSpan) DeleteNodes(nodeIDs []int32) { s.current().DeleteNodes(nodeIDs) }
func (s *syncSpan) GetTimeSpan(nodeID int32) (time.Time, time.Time, bool) {
	return s.current().GetTimeSpan(nodeID)
}
func (s *syncSpan) GetNodes() []model.NodeTimeSpan { return s.current().GetNodes() }

// startTimeSpanRefresher reloads the span tree on a fixed interval into holder.
// Failures are logged and the previous tree is kept, so a transient QuestDB or
// Postgres problem degrades freshness rather than losing the decorator entirely.
func startTimeSpanRefresher(holder *syncSpan, loader func(context.Context) ([]model.NodeTimeSpan, error), every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), every)
			rebuilt, err := buildSpanTree(ctx, loader)
			cancel()
			if err != nil {
				log.Errorf("timespan decorator: refresh failed, keeping previous tree: %v", err)
				continue
			}
			holder.set(rebuilt)
			log.Debugf("timespan decorator: refreshed %d nodes", len(rebuilt.GetNodes()))
		}
	}()
}

// buildSpanTree loads the tree from cfg.Loader. InitializeTree returns an
// already ordered and aggregated tree, so there is nothing left to fix up here.
func buildSpanTree(ctx context.Context, loader func(context.Context) ([]model.NodeTimeSpan, error)) (service.NodeTimeSpanServiceInterface, error) {
	nodes, err := loader(ctx)
	if err != nil {
		return nil, err
	}
	return service.InitializeTree(nodes)
}

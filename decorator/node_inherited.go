package decorator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/BayCEER/bayeos/bayeos-api/service"

	log "github.com/BayCEER/bayeos/bayeos-api/logger"
)

// InheritedConfig configures the InheritedProps decorator.
//
// The decorator has two halves. On the read path it copies properties an
// ancestor supplies into a node's metadata and marks them with
// model.InheritedKey. On the write path it refuses bodies carrying that marker,
// so a client echoing a node read cannot turn derived values into local ones.
type InheritedConfig struct {
	// Props is the in-memory property tree. Required.
	Props service.NodePropertyServiceInterface

	// Rules declare which node kinds supply inherited metadata. Required; with
	// no rules the decorator does nothing.
	Rules []service.PropertyRule

	// Loader rebuilds the tree from scratch. It is required when RefreshEvery
	// is set, and is also the source used for the initial load.
	Loader func(ctx context.Context) ([]model.Node, error)

	// SyncWrites keeps the tree in step with node mutations, so a value changed
	// or a subtree moved is reflected before the next refresh.
	SyncWrites bool

	// RefreshEvery periodically rebuilds the tree via Loader. It is the safety
	// net for mutations this decorator cannot observe, notably the Folder
	// created directly through the repository at sign-up and the intermediate
	// folders of path based creation, whose ids are never returned. Zero
	// disables the refresher.
	RefreshEvery time.Duration
}

// InheritedProps returns a Decorator that merges ancestor-supplied metadata
// properties into a node's own metadata.
//
// Inherited values are written into metadata rather than exposed as a separate
// field, so existing readers of unit, lat, lon and resolution keep working
// unchanged. Which of those values came from an ancestor is recorded in the
// model.InheritedKey marker, which is a list of the property names that were
// injected. The marker is set only when something was actually inherited, so a
// node with no inheritable metadata is returned byte-for-byte unchanged.
//
// A local value always wins: an inherited property is written only where the
// key is absent. When both the node and an ancestor set the same property, the
// node keeps its own value and the property is still listed as inherited,
// because it genuinely was resolved from the ancestor chain.
func InheritedProps(cfg InheritedConfig) Decorator {
	if cfg.Props == nil {
		panic("decorator: InheritedProps requires Props")
	}
	if len(cfg.Rules) == 0 {
		panic("decorator: InheritedProps requires at least one Rule")
	}

	// The holder is installed before the enricher and wrapper are built, so both
	// capture the swappable tree rather than the initial one.
	if cfg.RefreshEvery > 0 {
		if cfg.Loader == nil {
			panic("decorator: InheritedProps RefreshEvery requires a Loader")
		}
		holder := &syncProps{impl: cfg.Props}
		cfg.Props = holder
		startPropertyRefresher(holder, cfg.Loader, cfg.Rules, cfg.RefreshEvery)
	}

	d := Decorator{
		Name:   "inherited",
		Enrich: inheritedEnricher(cfg),
		// The wrapper is installed whether or not writes are being synced.
		// Refusing an echoed read is a contract with the client, not bookkeeping:
		// leaving it tied to SyncWrites would silently start persisting markers
		// and inherited values the moment write-sync was switched off. The tree
		// bookkeeping it otherwise owns no-ops when SyncWrites is false, so the
		// guard is all that remains.
		Wrap: func(next service.NodeService) service.NodeService {
			return &inheritedService{NodeService: next, cfg: cfg}
		},
	}
	return d
}

// inheritedEnricher copies inherited properties into the node's metadata and
// records which ones it wrote.
//
// Only written keys are recorded. A property the node already defined locally is
// left alone and left out of the marker, because the value the client sees came
// from the node: listing it would say "not yours to set" about a key it is
// perfectly entitled to change.
func inheritedEnricher(cfg InheritedConfig) func(context.Context, *View) {
	return func(_ context.Context, v *View) {
		inherited := cfg.Props.Inherited(v.ID())
		if len(inherited) == 0 {
			return
		}

		md := v.Metadata()
		for key, value := range inherited {
			// Local wins: an existing key is left exactly as it is.
			if _, exists := md[key]; exists {
				continue
			}
			md[key] = value
			model.AddMarkerKey(md, model.InheritedKey, key)
		}
	}
}

// inheritedService is the write-path half of the InheritedProps decorator. Every
// method it does not override is promoted from the wrapped service unchanged.
type inheritedService struct {
	service.NodeService
	cfg InheritedConfig
}

var _ service.NodeService = (*inheritedService)(nil)

// UpdateNode rejects a body carrying the inherited marker, then treats a
// parent_id change as a move and a metadata change as a local value change.
func (s *inheritedService) UpdateNode(ctx context.Context, id int32, fields *map[string]any) error {
	if err := rejectComputedFields(fields); err != nil {
		return err
	}

	if err := s.NodeService.UpdateNode(ctx, id, fields); err != nil {
		return err
	}
	if fields == nil {
		return nil
	}

	if raw, ok := (*fields)["parent_id"]; ok {
		if newParent, ok := toInt32(raw); ok && s.cfg.SyncWrites {
			s.cfg.Props.MoveNode(id, newParent)
		}
	}
	if raw, ok := (*fields)["metadata"]; ok {
		if md, ok := raw.(map[string]any); ok && s.cfg.SyncWrites {
			m, err := model.MetadataFromMap(md)
			if err == nil {
				s.cfg.Props.SetLocalMetadata(id, m)
			}
		}
	}
	return nil
}

// UpdateNodeMetadata rejects the marker and records the new local values.
func (s *inheritedService) UpdateNodeMetadata(ctx context.Context, id int32, metadata map[string]any) error {
	// This method receives the metadata document itself, so the marker sits at
	// the top level of the map rather than under a "metadata" key.
	if err := rejectComputedMetadata(metadata); err != nil {
		return err
	}

	if err := s.NodeService.UpdateNodeMetadata(ctx, id, metadata); err != nil {
		return err
	}
	if !s.cfg.SyncWrites {
		return nil
	}
	m, err := model.MetadataFromMap(metadata)
	if err != nil {
		return nil
	}
	s.cfg.Props.SetLocalMetadata(id, m)
	return nil
}

// CreateNode registers the new node. Its metadata is stored as local values
// only, because the node was just created and cannot already have inherited
// anything a client could have observed. A marker in the body is refused here
// for the same reason updates refuse one, even though the repository would
// strip it anyway.
func (s *inheritedService) CreateNode(ctx context.Context, node *model.Node, access model.NodeAccess) error {
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

	var metadata model.Metadata
	if node.Metadata.RawMessage != nil {
		metadata = node.Metadata
	}
	if s.cfg.SyncWrites {
		s.cfg.Props.AddNode(node.ID, node.ParentID, node.Kind, metadata)
	}
	return nil
}

// CreateNodeByPath is not tracked incrementally: the intermediate folders
// created along the path are not returned by the service, so their ids are
// unknown. The next refresh picks them up.
func (s *inheritedService) CreateNodeByPath(ctx context.Context, node *model.Node, baseNodeID int32, path string, access model.NodeAccess) error {
	if node != nil {
		if err := rejectComputedMetadata(node.Metadata.Map()); err != nil {
			return err
		}
	}
	return s.NodeService.CreateNodeByPath(ctx, node, baseNodeID, path, access)
}

// DeleteNodesByIDs passes the whole id set to the property service, which
// cascades over descendants in one pass.
func (s *inheritedService) DeleteNodesByIDs(ctx context.Context, ids []int32) error {
	if err := s.NodeService.DeleteNodesByIDs(ctx, ids); err != nil {
		return err
	}
	if s.cfg.SyncWrites {
		s.cfg.Props.DeleteNodes(ids)
	}
	return nil
}

// DeleteNode implements the singular delete for completeness.
func (s *inheritedService) DeleteNode(ctx context.Context, id int32) error {
	if err := s.NodeService.DeleteNode(ctx, id); err != nil {
		return err
	}
	if s.cfg.SyncWrites {
		s.cfg.Props.DeleteNodes([]int32{id})
	}
	return nil
}

// rejectComputedMetadata refuses a whole write whose metadata carries any
// computed marker, so the caller learns which key to drop instead of having its
// values silently discarded.
//
// Rejecting rather than stripping is deliberate. Stripping would silently drop
// the keys a marker names, so a client that edited an inherited value and
// submitted the object unchanged would have the edit discarded with no
// diagnostic. A 400 names the offending key, and because the markers are new no
// existing client sends them, so nothing that worked before now fails.
//
// The error is matched to the marker actually present, so the message stays
// specific about whether the values came from an ancestor or from a subtree
// roll-up.
func rejectComputedMetadata(md map[string]any) error {
	for _, marker := range model.ComputedMarkers {
		if _, present := md[marker]; present {
			return computedMarkerError(marker)
		}
	}
	return nil
}

// rejectComputedFields applies rejectComputedMetadata to a partial update body.
func rejectComputedFields(fields *map[string]any) error {
	if fields == nil {
		return nil
	}
	raw, ok := (*fields)["metadata"]
	if !ok {
		return nil
	}
	md, ok := raw.(map[string]any)
	if !ok {
		// The base service also accepts a JSON string or nil for metadata. Those
		// shapes are not the common case, and the repository strips the markers
		// from every metadata write whatever shape it arrives in, so passing them
		// through cannot persist them.
		return nil
	}
	return rejectComputedMetadata(md)
}

// computedMarkerError builds the error the handler turns into a 400.
func computedMarkerError(marker string) error {
	if marker == model.AggregatedKey {
		return fmt.Errorf("%w: %s in metadata", repository.ErrAggregatedNotWritable, marker)
	}
	return fmt.Errorf("%w: %s in metadata", repository.ErrInheritedNotWritable, marker)
}

// syncProps is a swappable holder around a property tree, so a periodic rebuild
// can replace the tree without racing the in-flight requests that read it.
type syncProps struct {
	mu   sync.RWMutex
	impl service.NodePropertyServiceInterface
}

func (p *syncProps) current() service.NodePropertyServiceInterface {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.impl
}

func (p *syncProps) set(impl service.NodePropertyServiceInterface) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.impl = impl
}

func (p *syncProps) Inherited(nodeID int32) map[string]any { return p.current().Inherited(nodeID) }
func (p *syncProps) SetLocalMetadata(nodeID int32, metadata model.Metadata) {
	p.current().SetLocalMetadata(nodeID, metadata)
}
func (p *syncProps) AddNode(nodeID int32, parentID int32, kind string, metadata model.Metadata) {
	p.current().AddNode(nodeID, parentID, kind, metadata)
}
func (p *syncProps) DeleteNodes(nodeIDs []int32) { p.current().DeleteNodes(nodeIDs) }
func (p *syncProps) MoveNode(nodeID int32, newParentID int32) {
	p.current().MoveNode(nodeID, newParentID)
}
func (p *syncProps) GetNodes() []model.Node { return p.current().GetNodes() }

// startPropertyRefresher reloads the property tree on a fixed interval into
// holder. Failures are logged and the previous tree is kept, so a transient
// Postgres problem degrades freshness rather than losing the decorator.
func startPropertyRefresher(holder *syncProps, loader func(context.Context) ([]model.Node, error), rules []service.PropertyRule, every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), every)
			rebuilt, err := buildPropertyTree(ctx, loader, rules)
			cancel()
			if err != nil {
				log.Errorf("inherited decorator: refresh failed, keeping previous tree: %v", err)
				continue
			}
			holder.set(rebuilt)
			log.Debugf("inherited decorator: refreshed %d nodes", len(rebuilt.GetNodes()))
		}
	}()
}

// buildPropertyTree loads and rebuilds the tree from loader.
func buildPropertyTree(ctx context.Context, loader func(context.Context) ([]model.Node, error), rules []service.PropertyRule) (service.NodePropertyServiceInterface, error) {
	nodes, err := loader(ctx)
	if err != nil {
		return nil, err
	}
	return service.InitializePropertyTree(nodes, rules)
}

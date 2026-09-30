package main

import (
	"context"
	"fmt"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/decorator"
	log "github.com/BayCEER/bayeos/bayeos-api/logger"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/BayCEER/bayeos/bayeos-api/service"
)

// refreshInterval is how often the trees are rebuilt in the background, so
// mutations that bypass the decorated service are picked up.
//
// The trees are keyed by node id, so a tree built before the database was
// restored or recreated maps an id onto the wrong ancestry and silently resolves
// no inherited values. Nothing on the read path can detect that, so the
// interval bounds how long it lasts. A failed refresh keeps the previous tree,
// so a shorter interval also means a transient database problem clears sooner.
//

const refreshInterval = 60 * time.Minute

// timeSpanKinds are the node kinds loaded into the time span tree. Every kind
// that can appear in the tree must be listed, otherwise a node whose kind is
// missing becomes an orphan and its data stops rolling up into its ancestors.
var timeSpanKinds = []string{model.KindFolder, model.KindTimeseries}

// propertyKinds are the node kinds loaded into the property tree. Inheritance
// needs the whole chain, so every kind that can appear must be listed: omitting
// one would sever the parent link above it and silently stop inheritance.
var propertyKinds = []string{model.KindFolder, model.KindTimeseries}

// propertyRules declare which node kinds supply inherited metadata to their
// descendants. Every key a Folder holds reaches every descendant that does not
// define it itself, so a whole subtree of series can share a single unit.
var propertyRules = []service.PropertyRule{
	{SourceKind: model.KindFolder},
}

// decorateNodes is the single place where cross-cutting node behaviour is
// declared. It is a static slice, evaluated once at start-up: there is no
// registry, no discovery and no runtime reconfiguration. Adding a capability
// is one line here, and removing the line removes the behaviour.
//
// Ordering is fixed by the package documentation of the decorator package:
// Wrap is applied outside-in in declaration order, and enrich runs after the
// wrapped call returns in declaration order.
func decorateNodes(base service.NodeService, span service.NodeTimeSpanServiceInterface, spanLoader func(context.Context) ([]model.NodeTimeSpan, error), props service.NodePropertyServiceInterface, propLoader func(context.Context) ([]model.Node, error)) service.NodeService {
	chain := []decorator.Decorator{
		decorator.TimeSpan(decorator.TimeSpanConfig{
			Span:          span,
			Loader:        spanLoader,
			Kinds:         timeSpanKinds,
			OmitWhenEmpty: true,
			SyncWrites:    true,
			RefreshEvery:  refreshInterval,
		}),
		decorator.InheritedProps(decorator.InheritedConfig{
			Props:        props,
			Rules:        propertyRules,
			Loader:       propLoader,
			SyncWrites:   true,
			RefreshEvery: refreshInterval,
		}),
	}

	log.Infof("active node decorators: %v", decorator.Names(chain))
	return decorator.Chain(base, chain...)
}

// newNodeTimeSpanService builds the initial time span tree. It goes through the
// same loader as the periodic refresh, so the first load and every later reload
// share one code path instead of two hand-kept copies of it. A failure is
// fatal: continuing would serve nodes with no time span at all.
func newNodeTimeSpanService(sqlRepo repository.SqlRepo, questRepo repository.QuestRepo) (service.NodeTimeSpanServiceInterface, error) {
	load := timeSpanLoader(sqlRepo, questRepo)

	start := time.Now()
	tsNodes, err := load(context.Background())
	if err != nil {
		return nil, fmt.Errorf("load node time spans: %w", err)
	}
	log.Debugf("loaded %d nodes in %s", len(tsNodes), time.Since(start).Round(time.Millisecond))

	start = time.Now()
	ts, err := service.InitializeTree(tsNodes)
	if err != nil {
		return nil, fmt.Errorf("initialize time span tree: %w", err)
	}
	log.Infof("tree built for %d nodes in %s", len(tsNodes), time.Since(start).Round(time.Millisecond))
	return ts, nil
}

// propertyLoader returns the reload function handed to the InheritedProps
// decorator, and is also the initial load path. Inheritance needs each node's
// own metadata, so the query selects it alongside the parent link.
func propertyLoader(sqlRepo repository.SqlRepo) func(context.Context) ([]model.Node, error) {
	return func(ctx context.Context) ([]model.Node, error) {
		return sqlRepo.GetAllNodesByKind(ctx, propertyKinds)
	}
}

// newNodePropertyService builds the initial property tree. It shares the loader
// with the periodic refresh so the first load and every later reload run the
// same code path.
func newNodePropertyService(sqlRepo repository.SqlRepo) (service.NodePropertyServiceInterface, error) {
	start := time.Now()
	nodes, err := propertyLoader(sqlRepo)(context.Background())
	if err != nil {
		return nil, fmt.Errorf("load node properties: %w", err)
	}

	props, err := service.InitializePropertyTree(nodes, propertyRules)
	if err != nil {
		return nil, fmt.Errorf("initialize property tree: %w", err)
	}
	log.Infof("property tree built for %d nodes in %s", len(nodes), time.Since(start).Round(time.Millisecond))
	return props, nil
}

// timeSpanLoader returns the reload function handed to the TimeSpan decorator,
// and is also the initial load path.
//
// Every node of a loaded kind is emitted with its parent link, so the tree stays
// connected. Only timeseries nodes carry local bounds; folders inherit their
// bounds from descendants during aggregation. A series whose range cannot be
// read is left without bounds and logged, so one bad series degrades to a
// missing bound rather than failing the whole load.
func timeSpanLoader(sqlRepo repository.SqlRepo, questRepo repository.QuestRepo) func(context.Context) ([]model.NodeTimeSpan, error) {
	return func(ctx context.Context) ([]model.NodeTimeSpan, error) {
		nodes, err := sqlRepo.GetAllNodesByKind(ctx, timeSpanKinds)
		if err != nil {
			return nil, err
		}

		tsNodes := make([]model.NodeTimeSpan, 0, len(nodes))
		start := time.Now()
		queried, failed := 0, 0

		log.Infof("timespan load: querying %d nodes", len(nodes))

		for _, node := range nodes {
			tsNode := model.NodeTimeSpan{ID: node.ID, ParentID: node.ParentID}
			if node.Kind == model.Kind {
				queried++
				first, last, err := questRepo.QueryTimeRange(ctx, node.ID)
				if err != nil {
					failed++
					log.Errorf("timespan load: node %d: %s", node.ID, err)
				} else {
					tsNode.LocalFirst = first
					tsNode.LocalLast = last
				}
			}
			tsNodes = append(tsNodes, tsNode)
		}

		log.Infof("timespan load: queried %d series in %s (%d failed)", queried, time.Since(start).Round(time.Millisecond), failed)
		return tsNodes, nil
	}
}

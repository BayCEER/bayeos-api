package decorator

import (
	"context"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"

	log "github.com/BayCEER/bayeos/bayeos-api/logger"
)

// enriched is the outermost wrapper installed by Chain when at least one
// decorator contributes an Enrich function. It is the single place that knows
// which service methods return nodes, so a change to the read surface of
// NodeService surfaces here rather than in every decorator.
//
// All other methods are promoted from the embedded service unchanged.
type enriched struct {
	service.NodeService
	enrichers []enricher
}

// GetNode implements service.NodeService.
func (e *enriched) GetNode(ctx context.Context, id int32, userID int32) (model.Node, error) {
	n, err := e.NodeService.GetNode(ctx, id, userID)
	if err != nil {
		return n, err
	}
	e.run(ctx, &n)
	return n, nil
}

// GetNodes implements service.NodeService.
func (e *enriched) GetNodes(ctx context.Context, ids []int32, userID int32) ([]model.Node, error) {
	nodes, err := e.NodeService.GetNodes(ctx, ids, userID)
	if err != nil {
		return nodes, err
	}
	e.runAll(ctx, nodes)
	return nodes, nil
}

// GetNodeChildren implements service.NodeService.
func (e *enriched) GetNodeChildren(ctx context.Context, id int32, userID int32) ([]model.Node, error) {
	nodes, err := e.NodeService.GetNodeChildren(ctx, id, userID)
	if err != nil {
		return nodes, err
	}
	e.runAll(ctx, nodes)
	return nodes, nil
}

// GetNodeParents implements service.NodeService.
func (e *enriched) GetNodeParents(ctx context.Context, id int32, userID int32) ([]model.Node, error) {
	nodes, err := e.NodeService.GetNodeParents(ctx, id, userID)
	if err != nil {
		return nodes, err
	}
	e.runAll(ctx, nodes)
	return nodes, nil
}

// FindNodesByAntPath implements service.NodeService.
func (e *enriched) FindNodesByAntPath(ctx context.Context, pattern string, kind string, skip int, limit int, userID int32) (int32, []model.Node, error) {
	total, nodes, err := e.NodeService.FindNodesByAntPath(ctx, pattern, kind, skip, limit, userID)
	if err != nil {
		return total, nodes, err
	}
	e.runAll(ctx, nodes)
	return total, nodes, nil
}

// runAll applies the enrich chain to every node of a slice. A nil or empty
// slice is left alone.
func (e *enriched) runAll(ctx context.Context, nodes []model.Node) {
	for i := range nodes {
		e.run(ctx, &nodes[i])
	}
}

// run applies every matching enricher to a single node and commits the
// resulting metadata. The node is only committed if at least one enricher
// actually ran, so a chain that selects nothing leaves the payload byte-for-byte
// identical.
func (e *enriched) run(ctx context.Context, n *model.Node) {
	if n == nil || len(e.enrichers) == 0 {
		return
	}

	v := NewView(n)
	applied := false
	for _, en := range e.enrichers {
		if !matches(en.when, v) {
			continue
		}
		applied = true
		e.apply(ctx, en, v)
	}
	if applied {
		v.Commit()
	}
}

// apply runs one enricher, recovering a panic. Enrich is fail-open: a broken
// decorator degrades the payload but must not fail the request, since the node
// data itself is already correct.
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

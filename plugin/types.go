package plugin

import (
	"context"

	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/trace"
)

// NodeDeleteHookFunc is invoked by the core API after a node has been deleted
// from the Postgres node table, with the id of the deleted node. Plugins use it
// to clean up related data that lives outside the node table (e.g. QuestDB).
type NodeDeleteHookFunc func(ctx context.Context, nodeID int32) error

// Host is the extension surface passed to each plugin at registration time.
// It exposes the secured router group (AuthUser + TraceUser already applied)
// and the shared services/infrastructure a plugin may need.
type Host struct {
	// Router is the authenticated route group on which plugins register
	// endpoints. AuthUser and TraceUser middleware are already applied.
	Router gin.IRoutes
	// Quest is the QuestDB connection (raw pgx), matching how core accesses it.
	Quest *pgx.Conn
	// NodeService is shared for node CRUD and permission lookups.
	NodeService service.NodeService
	// NodePrivService is shared for granting node privileges.
	NodePrivService service.NodePrivService
	// Tracer creates spans for plugin-side database operations.
	Tracer trace.Tracer

	nodeDeleteHooks []NodeDeleteHookFunc
}

// AddNodeDeleteHook registers a callback to be run when a node is deleted via
// the generic DELETE /api/nodes/{id} route. Hooks run in registration order.
func (h *Host) AddNodeDeleteHook(fn NodeDeleteHookFunc) {
	if fn != nil {
		h.nodeDeleteHooks = append(h.nodeDeleteHooks, fn)
	}
}

// NodeDeleteHooks returns a copy of the hooks registered so far.
func (h *Host) NodeDeleteHooks() []NodeDeleteHookFunc {
	out := make([]NodeDeleteHookFunc, len(h.nodeDeleteHooks))
	copy(out, h.nodeDeleteHooks)
	return out
}

// Plugin is the interface a plugin must implement to extend the API.
type Plugin interface {
	// Name uniquely identifies the plugin. Registering two plugins with the
	// same name is rejected.
	Name() string
	// Register wires the plugin's routes and hooks into the host.
	Register(h *Host) error
}
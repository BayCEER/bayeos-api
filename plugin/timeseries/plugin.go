package timeseries

import (
	"context"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/plugin"
)

// Plugin implements plugin.Plugin for the timeseries extension.
type Plugin struct{}

// Name returns the plugin's unique identifier.
func (p *Plugin) Name() string { return "timeseries" }

// Register wires the timeseries routes and the QuestDB node-delete hook into
// the host.
func (p *Plugin) Register(h *plugin.Host) error {
	if h.Quest == nil {
		return fmt.Errorf("timeseries plugin requires a QuestDB connection")
	}
	if h.NodeService == nil || h.NodePrivService == nil {
		return fmt.Errorf("timeseries plugin requires node services")
	}

	repo := NewQuestRepo(h.Quest, h.Tracer)
	svc := NewTimeseriesService(h.NodeService, h.NodePrivService, repo)
	hd := NewHandlers(svc)

	rwCheck := middleware.NodePermissionCheck(h.NodeService, model.READ_PERMISSION+model.WRITE_PERMISSION)

	r := h.Router
	r.POST("/api/timeseries", middleware.IdFromBody("parent_id", "NodeID"), rwCheck, hd.Create)
	r.GET("/api/timeseries/:id", middleware.IdFromPath("id", "NodeID"), rwCheck, hd.Get)
	r.PATCH("/api/timeseries/:id", middleware.IdFromPath("id", "NodeID"), rwCheck, hd.Update)
	r.DELETE("/api/timeseries/:id", middleware.IdFromPath("id", "NodeID"), rwCheck, hd.Delete)
	r.POST("/api/timeseries/:id/data", middleware.IdFromPath("id", "NodeID"), rwCheck, hd.AddData)
	r.GET("/api/timeseries/:id/data", middleware.IdFromPath("id", "NodeID"), rwCheck, hd.QueryData)

	// Clean up QuestDB data when a node is deleted via the generic node route.
	h.AddNodeDeleteHook(func(ctx context.Context, nodeID int32) error {
		return repo.Delete(ctx, nodeID)
	})

	return nil
}

// init self-registers the plugin so a blank import activates it.
func init() {
	plugin.Register(&Plugin{})
}
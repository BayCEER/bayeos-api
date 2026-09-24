package timeseries

import (
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/middleware"
	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/plugin"
)

// Plugin implements plugin.Plugin for the timeseries extension.
type Plugin struct{}

// Name returns the plugin's unique identifier.
func (p *Plugin) Name() string { return "timeseries" }

// Register wires the timeseries routes into the host.
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

	chkPerm := middleware.NodePermissionCheck(h.NodeService, model.READ_PERMISSION+model.WRITE_PERMISSION)

	r := h.Router
	r.POST("/timeseries", middleware.ResolvePathOrIdFromBody("parent_id", "node_path", "NodeID"), chkPerm, hd.Create)
	r.GET("/timeseries/data", hd.QueryDataBatch)
	r.POST("/timeseries/data", hd.AddDataBatch)
	r.GET("/timeseries/timerange", hd.GetTimeRange)
	r.GET("/timeseries/:id", middleware.IdFromPath("id", "NodeID"), chkPerm, hd.Get)
	r.PATCH("/timeseries/:id", middleware.IdFromPath("id", "NodeID"), chkPerm, hd.Update)
	r.DELETE("/timeseries/:id", middleware.IdFromPath("id", "NodeID"), chkPerm, hd.Delete)
	r.POST("/timeseries/:id/data", middleware.IdFromPath("id", "NodeID"), chkPerm, hd.AddData)
	r.GET("/timeseries/:id/data", middleware.IdFromPath("id", "NodeID"), chkPerm, hd.QueryData)
	r.GET("/timeseries/:id/stats", middleware.IdFromPath("id", "NodeID"), chkPerm, hd.GetStats)
	r.GET("/timeseries/:id/gaps", middleware.IdFromPath("id", "NodeID"), chkPerm, hd.GetGaps)

	return nil
}

// init self-registers the plugin so a blank import activates it.
func init() {
	plugin.Register(&Plugin{})
}

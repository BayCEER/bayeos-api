package chart

import (
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/plugin"
)

// Plugin implements plugin.Plugin for the chart extension. It provides CRUD
// for the authenticated user's own charts.
type Plugin struct{}

// Name returns the plugin's unique identifier.
func (p *Plugin) Name() string { return "charts" }

// Register wires the chart routes into the host.
func (p *Plugin) Register(h *plugin.Host) error {
	if h.SqlDB == nil {
		return fmt.Errorf("chart plugin requires a PostgreSQL connection")
	}

	repo := NewChartRepo(h.SqlDB)
	svc := NewChartService(repo)
	hd := NewChartHandlers(svc)

	// User charts are scoped to the authenticated user; ownership is enforced
	// in the service/repository by the user id from the JWT.
	r := h.Router
	r.GET("/charts", hd.GetCharts)
	r.POST("/charts", hd.CreateChart)
	r.GET("/charts/:id", hd.GetChart)
	r.PATCH("/charts/:id", hd.UpdateChart)
	r.DELETE("/charts/:id", hd.DeleteChart)

	return nil
}

// init self-registers the plugin so a blank import activates it.
func init() {
	plugin.Register(&Plugin{})
}

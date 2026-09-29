package handler

import (
	log "github.com/BayCEER/bayeos/bayeos-api/logger"
	m "github.com/BayCEER/bayeos/bayeos-api/middleware"
	mo "github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel/trace"
)

type Handler struct {
	UserService         service.UserService
	NodeService         service.NodeService
	NodeTokenService    service.NodeTokenService
	NodePrivService     service.NodePrivService
	IDTokenService      service.IDTokenService
	UserGroupService    service.UserGroupService
	ChartService        service.ChartService
	TimeseriesService   service.TimeseriesService
	NodeTimeSpanService service.NodeTimeSpanServiceInterface
}

type Config struct {
	AssetsPath string
	Router     *gin.Engine
	// BasePath is the URL prefix (context) under which every handler is
	// registered, e.g. "bayeos-api". Routes drop the legacy "/api" segment and
	// are mounted as /<BasePath>/<route>.
	BasePath            string
	UserService         service.UserService
	NodeService         service.NodeService
	NodeTokenService    service.NodeTokenService
	NodePrivService     service.NodePrivService
	IDTokenService      service.IDTokenService
	UserGroupService    service.UserGroupService
	ChartService        service.ChartService
	TimeseriesService   service.TimeseriesService
	NodeTimeSpanService service.NodeTimeSpanServiceInterface

	Tracer trace.Tracer

	// SqlDB is the PostgreSQL connection handed to plugins via plugin.Host.
	SqlDB *sqlx.DB

	// Quest is the QuestDB connection pool handed to plugins.
	Quest *pgxpool.Pool
}

func NewMainHandler(c *Config) {
	h := &Handler{
		UserService:         c.UserService,
		NodeService:         c.NodeService,
		NodeTokenService:    c.NodeTokenService,
		NodePrivService:     c.NodePrivService,
		IDTokenService:      c.IDTokenService,
		UserGroupService:    c.UserGroupService,
		ChartService:        c.ChartService,
		TimeseriesService:   c.TimeseriesService,
		NodeTimeSpanService: c.NodeTimeSpanService,
	}

	r := c.Router
	base := "/" + c.BasePath

	// errors raised via mw.Abort/c.Error must be converted into the ErrorResponse body before any route handlers run.
	r.Use(m.Middleware())

	log.Debugf("serving static assets from:%v on %v/assets", c.AssetsPath, base)
	r.Use(static.Serve(base+"/assets", static.LocalFile(c.AssetsPath, false)))

	// Public
	r.POST(base+"/auth/signin", h.SignIn)
	r.POST(base+"/auth/signup", h.SignUp)

	// Secured by AuthUser middleware, which validates the ID token and adds the user to the context.
	sec := r.Group(base).Use(m.AuthUser(c.IDTokenService), m.TraceUser())
	sec.PUT("/auth/refresh", h.RefreshIDToken)
	sec.PUT("/auth/password", h.SetPassword)
	sec.POST("/auth/signout", h.SignOut)

	// Users
	sec.GET("/users", h.GetUsers)
	sec.GET("/users/:id", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.GetUser)

	sec.GET("/users/:id/image", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.GetUserImage)
	sec.POST("/users/:id/image", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.CreateUserImage)
	sec.DELETE("/users/:id/image", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.DeleteUserImage)

	sec.GET("/users/:id/privileges", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.GetUserPrivileges)

	sec.POST("/users", m.AuthSuper(), h.CreateUser)

	sec.PATCH("/users/:id", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.UpdateUser)
	sec.DELETE("/users/:id", m.AuthSuper(), h.DeleteUser)

	// User group
	sec.GET("/usergroups", m.AuthSuper(), h.GetUserGroups)
	sec.POST("/usergroups", m.AuthSuper(), h.CreateUserGroup)

	sec.GET("/usergroups/:id", m.AuthSuper(), h.GetUserGroup)
	sec.PATCH("/usergroups/:id", m.AuthSuper(), h.UpdateUserGroup)
	sec.DELETE("/usergroups/:id", m.AuthSuper(), h.DeleteUserGroup)

	sec.GET("/usergroups/:id/members", m.AuthSuper(), h.GetUserGroupMembers)
	sec.POST("/usergroups/:id/members", m.AuthSuper(), h.AddUserGroupMembers)
	sec.DELETE("/usergroups/:id/members/:uid", m.AuthSuper(), h.RemoveUserGroupMember)

	sec.GET("/usergroups/:id/privileges", m.AuthSuper(), h.GetUserGroupPrivileges)

	// Node
	sec.GET("/nodes/search", h.SearchNodes)
	sec.GET("/nodes", h.GetNodes)

	rwCheck := m.NodePermissionCheck(h.NodeService, mo.READ_PERMISSION+mo.WRITE_PERMISSION)
	rwgCheck := m.NodePermissionCheck(h.NodeService, mo.FULL_PERMISSION)

	sec.POST("/nodes", m.ResolvePathOrIdFromBody("parent_id", "node_path", "NodeID"), rwCheck, h.CreateNode)
	sec.GET("/nodes/:id/children", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodeChildren)
	sec.GET("/nodes/:id/parents", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodeParents)
	sec.GET("/nodes/:id", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNode)
	sec.PATCH("/nodes/:id", m.IdFromPath("id", "NodeID"), rwCheck, h.UpdateNode)
	sec.DELETE("/nodes/:id", m.IdFromPath("id", "NodeID"), rwCheck, h.DeleteNode)
	sec.GET("/nodes/:id/tokens", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodeTokens)
	sec.POST("/nodes/:id/tokens", m.IdFromPath("id", "NodeID"), rwCheck, h.CreateNodeToken)
	sec.GET("/nodes/:id/tokens/:nid", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodeToken)
	sec.DELETE("/nodes/:id/tokens/:nid", m.IdFromPath("id", "NodeID"), rwCheck, h.DeleteNodeToken)
	sec.PATCH("/nodes/:id/tokens/:nid", m.IdFromPath("id", "NodeID"), rwCheck, h.UpdateNodeToken)
	sec.GET("/nodes/:id/privileges", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodePrivs)
	sec.POST("/nodes/:id/privileges", m.IdFromPath("id", "NodeID"), rwgCheck, h.CreateNodePriv)
	sec.GET("/nodes/:id/privileges/:pid", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodePriv)
	sec.PATCH("/nodes/:id/privileges/:pid", m.IdFromPath("id", "NodeID"), rwgCheck, h.UpdateNodePriv)
	sec.DELETE("/nodes/:id/privileges/:pid", m.IdFromPath("id", "NodeID"), rwgCheck, h.DeleteNodePrivilege)

	// Timeseries
	sec.POST("/timeseries", m.ResolvePathOrIdFromBody("parent_id", "node_path", "NodeID"), rwCheck, h.CreateTimeseries)
	sec.GET("/timeseries/data", h.QueryTimeseriesDataBatch)
	sec.POST("/timeseries/data", h.AddTimeseriesDataBatch)
	sec.GET("/timeseries/:id", m.IdFromPath("id", "NodeID"), rwCheck, h.GetTimeseries)
	sec.PATCH("/timeseries/:id", m.IdFromPath("id", "NodeID"), rwCheck, h.UpdateTimeseries)
	sec.DELETE("/timeseries/:id", m.IdFromPath("id", "NodeID"), rwCheck, h.DeleteTimeseries)
	sec.POST("/timeseries/:id/data", m.IdFromPath("id", "NodeID"), rwCheck, h.AddTimeseriesData)
	sec.GET("/timeseries/:id/data", m.IdFromPath("id", "NodeID"), rwCheck, h.QueryTimeseriesData)
	sec.GET("/timeseries/:id/stats", m.IdFromPath("id", "NodeID"), rwCheck, h.GetTimeseriesStats)
	sec.GET("/timeseries/:id/gaps", m.IdFromPath("id", "NodeID"), rwCheck, h.GetTimeseriesGaps)

	// Chart
	sec.GET("/charts", h.GetCharts)
	sec.POST("/charts", h.CreateChart)
	sec.GET("/charts/:id", h.GetChart)
	sec.PATCH("/charts/:id", h.UpdateChart)
	sec.DELETE("/charts/:id", h.DeleteChart)

}

package handler

import (
	"context"

	log "github.com/BayCEER/bayeos/bayeos-api/logger"
	m "github.com/BayCEER/bayeos/bayeos-api/middleware"
	mo "github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/plugin"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/trace"
)

type Handler struct {
	UserService      service.UserService
	NodeService      service.NodeService
	NodeTokenService service.NodeTokenService
	NodePrivService  service.NodePrivService
	IDTokenService   service.IDTokenService

	nodeDeleteHooks []plugin.NodeDeleteHookFunc
}

type Config struct {
	AssetsPath       string
	Router           *gin.Engine
	UserService      service.UserService
	NodeService      service.NodeService
	NodeTokenService service.NodeTokenService
	NodePrivService  service.NodePrivService
	IDTokenService   service.IDTokenService
	Tracer           trace.Tracer

	// Quest is the QuestDB connection handed to plugins.
	Quest *pgx.Conn
	// Plugins are registered via NewMainHandler after the secured route
	// group is built.
	Plugins []plugin.Plugin
}

func NewMainHandler(c *Config) {
	h := &Handler{
		UserService:      c.UserService,
		NodeService:      c.NodeService,
		NodeTokenService: c.NodeTokenService,
		NodePrivService:  c.NodePrivService,
		IDTokenService:   c.IDTokenService,
	}

	r := c.Router

	// errors raised via mw.Abort/c.Error must be converted into the ErrorResponse body before any route handlers run.
	r.Use(m.Middleware())

	log.Debugf("serving static assets from:%v on /api/assets", c.AssetsPath)
	r.Use(static.Serve("/api/assets", static.LocalFile(c.AssetsPath, false)))

	// Public
	r.POST("/api/auth/signin", h.SignIn)
	r.POST("/api/auth/signup", h.SignUp)

	sec := r.Group("/").Use(m.AuthUser(c.IDTokenService), m.TraceUser())
	sec.PUT("/api/auth/refresh", h.RefreshIDToken)
	sec.PUT("/api/auth/password", h.SetPassword)
	sec.POST("/api/auth/signout", h.SignOut)

	sec.GET("/api/users", h.GetUsers)
	sec.GET("/api/users/:id", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.GetUser)

	sec.GET("/api/users/:id/image", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.GetUserImage)
	sec.POST("/api/users/:id/image", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.CreateUserImage)
	sec.DELETE("/api/users/:id/image", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.DeleteUserImage)

	sec.GET("/api/users/:id/privileges/grantor", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.GetNodePrivsByGrantorID)
	sec.GET("/api/users/:id/privileges/grantee", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.GetNodePrivsByGranteeID)

	sec.POST("/api/users", m.AuthSuper(), h.CreateUser)

	sec.PATCH("/api/users/:id", m.IdFromPath("id", "UserID"), m.UserPermissionCheck(), h.UpdateUser)
	sec.DELETE("/api/users/:id", m.AuthSuper(), h.DeleteUser)

	// With permission check implemented in handler
	sec.GET("/api/nodes/search", h.SearchNodes)
	sec.GET("/api/nodes/search/path", h.SearchNodesByPath)
	sec.GET("/api/nodes", h.GetNodes)

	// Node Permissions
	rwCheck := m.NodePermissionCheck(h.NodeService, mo.READ_PERMISSION+mo.WRITE_PERMISSION)
	rwgCheck := m.NodePermissionCheck(h.NodeService, mo.FULL_PERMISSION)

	sec.POST("/api/nodes", m.IdFromBody("parent_id", "NodeID"), rwCheck, h.CreateNode)

	sec.GET("/api/nodes/:id/children", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodeChildren)
	sec.GET("/api/nodes/:id/parents", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodeParents)

	sec.GET("/api/nodes/:id", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNode)
	sec.PATCH("/api/nodes/:id", m.IdFromPath("id", "NodeID"), rwCheck, h.UpdateNode)
	sec.DELETE("/api/nodes/:id", m.IdFromPath("id", "NodeID"), rwCheck, h.DeleteNode)

	sec.GET("/api/nodes/:id/tokens", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodeTokens)
	sec.POST("/api/nodes/:id/tokens", m.IdFromPath("id", "NodeID"), rwCheck, h.CreateNodeToken)

	sec.GET("/api/nodes/:id/tokens/:nid", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodeToken)
	sec.DELETE("/api/nodes/:id/tokens/:nid", m.IdFromPath("id", "NodeID"), rwCheck, h.DeleteNodeToken)
	sec.PATCH("/api/nodes/:id/tokens/:nid", m.IdFromPath("id", "NodeID"), rwCheck, h.UpdateNodeToken)

	sec.GET("/api/nodes/:id/privileges", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodePrivs)
	sec.POST("/api/nodes/:id/privileges", m.IdFromPath("id", "NodeID"), rwgCheck, h.CreateNodePriv)

	sec.GET("/api/nodes/:id/privileges/:pid", m.IdFromPath("id", "NodeID"), rwCheck, h.GetNodePriv)
	sec.PATCH("/api/nodes/:id/privileges/:pid", m.IdFromPath("id", "NodeID"), rwgCheck, h.UpdateNodePriv)
	sec.DELETE("/api/nodes/:id/privileges/:pid", m.IdFromPath("id", "NodeID"), rwgCheck, h.DeleteNodePrivilege)

	registerPlugins(h, c, sec)
}

// registerPlugins hands each plugin the secured route group and shared
// infrastructure, then stores any node-delete hooks they register.
func registerPlugins(h *Handler, c *Config, sec gin.IRoutes) {
	host := &plugin.Host{
		Router:          sec,
		Quest:           c.Quest,
		NodeService:     c.NodeService,
		NodePrivService: c.NodePrivService,
		Tracer:          c.Tracer,
	}
	for _, p := range c.Plugins {
		if p == nil {
			continue
		}
		if err := p.Register(host); err != nil {
			log.Errorf("plugin %q registration failed: %v", p.Name(), err)
			continue
		}
		h.nodeDeleteHooks = append(h.nodeDeleteHooks, host.NodeDeleteHooks()...)
		log.Infof("plugin %q registered", p.Name())
	}
}

// runNodeDeleteHooks invokes all registered node-delete hooks after a node has
// been removed from the Postgres node table. Errors are logged but do not fail
// the already-committed node deletion.
func (h *Handler) runNodeDeleteHooks(ctx context.Context, nodeID int32) {
	for _, hook := range h.nodeDeleteHooks {
		if err := hook(ctx, nodeID); err != nil {
			log.Errorf("node delete hook failed for node %d: %v", nodeID, err)
		}
	}
}

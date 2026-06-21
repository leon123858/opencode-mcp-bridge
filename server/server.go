package server

import (
	"net/http"

	"github.com/a0970/opencodemcpbridge/handlers"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func New(h *handlers.Handler, mcpHandler ...http.Handler) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

	e.GET("/healthz", func(c echo.Context) error { return c.JSON(http.StatusOK, map[string]string{"status": "ok"}) })
	if len(mcpHandler) > 0 && mcpHandler[0] != nil {
		wrapped := echo.WrapHandler(mcpHandler[0])
		e.Any("/", wrapped)
		e.Any("/mcp", wrapped)
	}
	g := e.Group("/opencode")
	g.GET("/setup", h.Setup)
	g.POST("/ask", h.Ask)
	g.POST("/reply", h.Reply)
	g.POST("/run", h.Run)
	g.GET("/check", h.Check)
	g.GET("/conversation", h.Conversation)
	g.GET("/sessions-overview", h.SessionsOverview)
	g.GET("/mcp-servers", h.MCPServers)
	g.POST("/provider-test", h.ProviderTest)
	return e
}

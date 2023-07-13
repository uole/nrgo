package nrgo

import (
	"git.nspix.com/golang/kos"
	"git.nspix.com/golang/kos/entry/http"
	"github.com/uole/nrgo/version"
)

func (svr *Server) handleIndexRoute(ctx *http.Context) (err error) {
	return ctx.Success("OK")
}

func (svr *Server) handleNodeInfo(ctx *http.Context) (err error) {
	return ctx.Success(svr.info)
}

func (svr *Server) handleListSessions(ctx *http.Context) (err error) {
	return ctx.Success(svr.getSessionSnapshot())
}

func (svr *Server) handleShowVersion(ctx *http.Context) (err error) {
	return ctx.Success(map[string]any{
		"version":   version.Version,
		"buildDate": version.BuildDate,
	})
}

func (svr *Server) routes() {
	kos.Http().Handle(http.MethodGet, "/", svr.handleIndexRoute)
	kos.Http().Group("/api/v1", []http.Route{
		{http.MethodGet, "/info", svr.handleNodeInfo},
		{http.MethodGet, "/version", svr.handleShowVersion},
		{http.MethodGet, "/sessions", svr.handleListSessions},
	})
}

package nrgo

import (
	"git.nspix.com/golang/kos"
	"git.nspix.com/golang/kos/entry/http"
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

func (svr *Server) routes() {
	kos.Http().Handle(http.MethodGet, "/", svr.handleIndexRoute)
	kos.Http().Group("/api/v1", []http.Route{
		{http.MethodGet, "/info", svr.handleNodeInfo},
		{http.MethodGet, "/sessions", svr.handleListSessions},
	})
}

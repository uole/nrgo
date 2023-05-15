package nrgo

import (
	"git.nspix.com/golang/kos"
	"git.nspix.com/golang/kos/entry/http"
)

const (
	ErrHttpRenew = 0x4001
)

func (svr *Server) handleNodeInfo(ctx *http.Context) (err error) {
	return ctx.Success(svr.info)
}

func (svr *Server) handleListSessions(ctx *http.Context) (err error) {
	return ctx.Success(svr.readSessionSnapshot())
}

func (svr *Server) handleRenewIP(ctx *http.Context) (err error) {
	if err = svr.wireguard.Reload(); err != nil {
		return ctx.Error(ErrHttpRenew, err.Error())
	}
	return ctx.Success("OK")
}

func (svr *Server) routes() {
	kos.Http().Group("/api/v1", []http.Route{
		{http.MethodGet, "/info", svr.handleNodeInfo},
		{http.MethodGet, "/renew", svr.handleRenewIP},
		{http.MethodGet, "/sessions", svr.handleListSessions},
	})
}

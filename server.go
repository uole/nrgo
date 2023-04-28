package nrgo

import (
	"context"
	"errors"
	"fmt"
	"git.nspix.com/golang/kos/pkg/log"
	"git.nspix.com/golang/kos/util/env"
	"git.nspix.com/golang/kos/util/fetch"
	"git.nspix.com/golang/kos/util/sys"
	retry "github.com/avast/retry-go"
	"github.com/rs/xid"
	"github.com/sourcegraph/conc"
	"github.com/uole/nrgo/version"
	"os"
	"path"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

type Server struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	Uptime     time.Time
	cfg        *Config
	info       *NodeInfo
	serveInfo  *ServeInfo
	waitGroup  conc.WaitGroup
	mutex      sync.RWMutex
	sessions   map[string]*Session
}

func (svr *Server) readSessionSnapshot() []*Session {
	svr.mutex.RLock()
	defer svr.mutex.RUnlock()
	ss := make([]*Session, 0, len(svr.sessions))
	for _, sess := range svr.sessions {
		ss = append(ss, sess)
	}
	return ss
}

func (svr *Server) domainName() string {
	return env.Get("NRGO_DOMAIN", svr.cfg.Domain)
}

func (svr *Server) extGeoInfo(ctx context.Context) (info *GeoInfo, err error) {
	info = &GeoInfo{}
	err = retry.Do(func() error {
		return fetch.Request(
			ctx,
			env.Get("NRGO_GEO_URL", "https://api.ip.sb/geoip"),
			info,
			fetch.WithHuman(),
		)
	},
		retry.Attempts(3),
		retry.Context(svr.ctx),
	)
	return
}

func (svr *Server) initConfig(ctx context.Context) (err error) {
	cfg := &Config{}
	return retry.Do(func() error {
		if err = fetch.Request(ctx, env.Get("NRGO_CONFIG_URL", "https://s3.tebi.io/nobla/vrgo.json"), cfg); err != nil {
			return err
		}
		svr.cfg = cfg
		return nil
	},
		retry.Attempts(5),
		retry.Context(svr.ctx),
	)
}

func (svr *Server) prepare(ctx context.Context) (err error) {
	var (
		buf []byte
		geo *GeoInfo
	)
	if err = svr.initConfig(svr.ctx); err != nil {
		return
	}
	if geo, err = svr.extGeoInfo(ctx); err == nil {
		svr.info.Country = geo.CountryCode
		svr.info.IP = geo.IP
	}
	svr.info.CPU = runtime.NumCPU()
	svr.info.Name, _ = os.Hostname()
	idFile := path.Join(sys.HomeDir(), sys.HiddenFile(version.ProductName))
	if buf, err = os.ReadFile(idFile); err == nil {
		svr.info.ID = string(buf)
	} else {
		svr.info.ID = xid.New().String()
		err = os.WriteFile(idFile, []byte(svr.info.ID), 0644)
	}
	return
}

func (svr *Server) lookupServeInfo(ctx context.Context) (err error) {
	res := &serveInfoResponse{}
	if err = retry.Do(func() error {
		if err = fetch.Request(ctx, fmt.Sprintf("https://%s/api/v1/info", svr.domainName()), res); err != nil {
			return err
		}
		if res.Code != 0 {
			return errors.New(res.Reason)
		}
		svr.serveInfo = &res.Data
		return nil
	},
		retry.Attempts(3),
		retry.Context(svr.ctx),
	); err != nil {
		return
	}
	svr.mutex.Lock()
	defer svr.mutex.Unlock()
	if len(svr.serveInfo.Slaves) > 0 {
		for _, slave := range svr.serveInfo.Slaves {
			if sess, ok := svr.sessions[slave.ID]; ok {
				sess.updateAddress(slave.Address)
			} else {
				sess = newSession(slave.ID, slave.Address, svr.info)
				svr.sessions[sess.ID] = sess
			}
		}
	} else {
		if sess, ok := svr.sessions[svr.serveInfo.ID]; ok {
			sess.updateAddress(svr.serveInfo.Address)
		} else {
			sess = newSession(svr.serveInfo.ID, svr.serveInfo.Address, svr.info)
			svr.sessions[sess.ID] = sess
		}
	}
	return
}

func (svr *Server) checkStatus() {
	var (
		err error
	)
	svr.mutex.RLock()
	defer func() {
		svr.mutex.RUnlock()
		if r := recover(); r != nil {
			log.Warn("runtime panic %v: %s", r, string(debug.Stack()))
		}
	}()
	for _, sess := range svr.sessions {
		if sess.IsEqual(StateReady) {
			sess.Ping(svr.ctx)
			duration := time.Now().Sub(sess.HeartbeatTime)
			if duration > time.Minute*3 {
				log.Warnf("session %s heartbeat timeout %s", sess.ID, duration)
				if err = sess.Close(); err != nil {
					log.Warnf("session %s close timeout connection error: %s", sess.ID, err.Error())
				}
			}
		} else {
			if err = sess.Connect(svr.ctx); err != nil {
				log.Warnf("session %s connect error: %s", sess.ID, err.Error())
			} else {
				log.Infof("session %s connect successful", sess.ID)
				go sess.Receive(svr.ctx)
			}
		}
	}
}

func (svr *Server) eventLoop() {
	ticker := time.NewTicker(time.Second * 60)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			svr.checkStatus()
		case <-svr.ctx.Done():
			return
		}
	}
}

func (svr *Server) Start(ctx context.Context) (err error) {
	os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")
	os.Setenv("QUIC_GO_LOG_LEVEL", "error")
	svr.ctx, svr.cancelFunc = context.WithCancel(ctx)
	if err = svr.prepare(svr.ctx); err != nil {
		return
	}
	if err = svr.lookupServeInfo(svr.ctx); err != nil {
		return
	}
	svr.routes()
	svr.waitGroup.Go(svr.checkStatus)
	svr.waitGroup.Go(svr.eventLoop)
	return
}

func (svr *Server) Stop() (err error) {
	svr.cancelFunc()
	return
}

func New() *Server {
	svr := &Server{
		info:     &NodeInfo{Uptime: time.Now()},
		Uptime:   time.Now(),
		sessions: make(map[string]*Session),
	}
	return svr
}

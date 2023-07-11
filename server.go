package nrgo

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"git.nspix.com/golang/kos/pkg/log"
	"git.nspix.com/golang/kos/util/env"
	"git.nspix.com/golang/kos/util/fetch"
	"git.nspix.com/golang/kos/util/sys"
	retry "github.com/avast/retry-go"
	"github.com/rs/xid"
	"github.com/sourcegraph/conc"
	"github.com/uole/nrgo/config"
	"github.com/uole/nrgo/internal/utils"
	"github.com/uole/nrgo/pkg/multiplex"
	"github.com/uole/nrgo/pkg/packet"
	"github.com/uole/nrgo/pkg/stream"
	"github.com/uole/nrgo/version"
	"golang.org/x/crypto/pbkdf2"
	"io"
	"net"
	"os"
	"path"
	"runtime"
	"sync"
	"time"
)

type Server struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	Uptime     time.Time
	remoteCf   *remoteConfig
	info       *NodeInfo
	serveInfo  *ServeInfo
	waitGroup  conc.WaitGroup
	mutex      sync.RWMutex
	secretKey  []byte
	checking   int32
	wireguard  *Wireguard
	cfg        *config.Config
	sessions   map[string]*Session
}

func (svr *Server) handshake(request *packet.Frame, stream multiplex.Stream) (rwc io.ReadWriteCloser, err error) {
	var (
		dialer  net.Dialer
		timeout time.Duration
	)
	req := &packet.HandshakeRequest{}
	if err = json.Unmarshal(request.Buf, req); err != nil {
		return
	}
	timeout = req.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	res := &packet.HandshakeResponse{
		Host: req.Host,
	}
	ctx, cancelFunc := context.WithTimeout(svr.ctx, timeout)
	defer cancelFunc()
	if !svr.cfg.Direct && svr.wireguard.Ready() {
		rwc, err = svr.wireguard.DialContext(ctx, "tcp", req.Host)
	} else {
		rwc, err = dialer.DialContext(ctx, "tcp", req.Host)
	}
	if err == nil {
		res.Success = true
		err = packet.WriteFrame(stream, packet.NewFrame(packet.TypeHandshakeResponse, request.Sequence, res))
	} else {
		res.Reason = err.Error()
		_ = packet.WriteFrame(stream, packet.NewFrame(packet.TypeHandshakeResponse, request.Sequence, res))
		log.Debugf("handshake %s error: %s", req.Host, err.Error())
	}
	return
}

func (svr *Server) getSessionSnapshot() []*Session {
	svr.mutex.RLock()
	defer svr.mutex.RUnlock()
	ss := make([]*Session, 0, len(svr.sessions))
	for _, sess := range svr.sessions {
		ss = append(ss, sess)
	}
	return ss
}

func (svr *Server) domainName() string {
	return env.Get("NRGO_DOMAIN", svr.remoteCf.Domain)
}

func (svr *Server) lookupGeoInfo(ctx context.Context) (info *GeoInfo, err error) {
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
	rc := &remoteConfig{}
	return retry.Do(func() error {
		if err = fetch.Request(ctx, env.Get("NRGO_CONFIG_URL", "https://s3.tebi.io/nobla/vrgo.json"), rc); err != nil {
			return err
		}
		svr.remoteCf = rc
		return nil
	},
		retry.Attempts(5),
		retry.Context(svr.ctx),
	)
}

func (svr *Server) initialization(ctx context.Context) (err error) {
	var (
		buf []byte
		geo *GeoInfo
	)
	if err = svr.initConfig(svr.ctx); err != nil {
		return
	}
	if geo, err = svr.lookupGeoInfo(ctx); err == nil {
		svr.info.Country = geo.CountryCode
		svr.info.IP = geo.IP
	}
	svr.info.OS = runtime.GOOS
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

func (svr *Server) refreshServeInfo(ctx context.Context) (err error) {
	var (
		secretKey string
	)
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
	if secretKey, err = utils.DecryptSecret(res.Data.SecretKey); err != nil {
		return
	}
	svr.secretKey = pbkdf2.Key([]byte(secretKey), utils.Salt, 4, stream.BlockSize, sha1.New)
	svr.mutex.Lock()
	defer svr.mutex.Unlock()
	if len(svr.serveInfo.Slaves) > 0 {
		for _, slave := range svr.serveInfo.Slaves {
			ci := &ConnectionInfo{
				Proto:     slave.Proto,
				ConnSize:  slave.ConnSize,
				Address:   slave.Address,
				SecretKey: svr.secretKey,
			}
			if sess, ok := svr.sessions[slave.ID]; ok {
				if err = sess.updateConnectionInfo(ctx, ci); err != nil {
					log.Warnf("update session %s connection info error: %s", sess.ID, err.Error())
				}
			} else {
				sess = newSession(slave.ID, ci, svr.info, sess.handshake)
				svr.sessions[sess.ID] = sess
				err = sess.Serve(ctx)
			}
		}
	} else {
		ci := &ConnectionInfo{
			Proto:     svr.serveInfo.Proto,
			ConnSize:  svr.serveInfo.ConnSize,
			Address:   svr.serveInfo.Address,
			SecretKey: svr.secretKey,
		}
		if sess, ok := svr.sessions[svr.serveInfo.ID]; ok {
			if err = sess.updateConnectionInfo(ctx, ci); err != nil {
				log.Warnf("update session %s connection info error: %s", sess.ID, err.Error())
			}
		} else {
			sess = newSession(svr.serveInfo.ID, ci, svr.info, svr.handshake)
			svr.sessions[sess.ID] = sess
			err = sess.Serve(ctx)
		}
	}
	return
}

func (svr *Server) eventLoop() {
	refreshTicker := time.NewTicker(time.Minute * 20)
	defer func() {
		refreshTicker.Stop()
	}()
	for {
		select {
		case <-refreshTicker.C:
			if err := svr.refreshServeInfo(svr.ctx); err != nil {
				log.Warnf("refresh server info failed cause by %s", err.Error())
			}
		case <-svr.ctx.Done():
			return
		}
	}
}

func (svr *Server) Start(ctx context.Context) (err error) {
	svr.ctx, svr.cancelFunc = context.WithCancel(ctx)
	if err = svr.initialization(svr.ctx); err != nil {
		return
	}
	if err = svr.refreshServeInfo(svr.ctx); err != nil {
		return
	}
	if !svr.cfg.Direct {
		svr.wireguard = newWireguard(svr.cfg.Wireguard)
		if err = svr.wireguard.Mount(svr.ctx); err != nil {
			log.Debugf("mount warp endpoint error: %s", err.Error())
			err = nil
		}
	}
	svr.routes()
	svr.waitGroup.Go(svr.eventLoop)
	return
}

func (svr *Server) Stop() (err error) {
	svr.cancelFunc()
	if !svr.cfg.Direct {
		if svr.wireguard != nil {
			err = svr.wireguard.Umount()
		}
	}
	for _, sess := range svr.sessions {
		err = sess.Close()
	}
	return
}

func New(cfg *config.Config) *Server {
	svr := &Server{
		cfg:      cfg,
		info:     &NodeInfo{Uptime: time.Now()},
		Uptime:   time.Now(),
		sessions: make(map[string]*Session),
	}
	return svr
}

package nrgo

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"git.nspix.com/golang/kos/pkg/log"
	"github.com/uole/nrgo/config"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
)

type (
	Wireguard struct {
		device    *device.Device
		netstack  *netstack.Net
		readyFlag int32
		mutex     sync.Mutex
		Config    config.Wireguard `json:"config" yaml:"config"`
	}
)

func (d *Wireguard) Ready() bool {
	return atomic.LoadInt32(&d.readyFlag) == 1
}

func (d *Wireguard) parseNetIP(address string) ([]netip.Addr, error) {
	var ips []netip.Addr
	for _, str := range strings.Split(address, ",") {
		str = strings.TrimSpace(str)
		if len(str) == 0 {
			continue
		}
		ip, err := netip.ParseAddr(str)
		if err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}
	return ips, nil
}

func (d *Wireguard) parseCIDRNetIP(address string) ([]netip.Addr, error) {
	var ips []netip.Addr
	for _, str := range strings.Split(address, ",") {
		str = strings.TrimSpace(str)
		if len(str) == 0 {
			continue
		}
		prefix, err := netip.ParsePrefix(str)
		if err != nil {
			return nil, err
		}
		addr := prefix.Addr()
		if prefix.Bits() != addr.BitLen() {
			return nil, errors.New("interface address subnet should be /32 for IPv4 and /128 for IPv6")
		}
		ips = append(ips, addr)
	}
	return ips, nil
}

func (d *Wireguard) parseBase64KeyToHex(value string) (str string, err error) {
	var (
		buf []byte
	)
	if buf, err = base64.StdEncoding.DecodeString(value); err != nil {
		return
	}
	if len(buf) != 32 {
		err = fmt.Errorf("key should be 32 bytes")
		return
	}
	str = hex.EncodeToString(buf)
	return
}

func (d *Wireguard) parseInterface(i config.Interface) (address, dns []netip.Addr, mtu uint, err error) {
	if address, err = d.parseCIDRNetIP(i.Address); err != nil {
		return
	}
	if dns, err = d.parseNetIP(i.DNS); err != nil {
		return
	}
	if i.MTU <= 0 {
		mtu = 1420
	} else {
		mtu = i.MTU
	}
	return
}

func (d *Wireguard) buildPeer(i config.Peer) (str string, err error) {
	var (
		sb        strings.Builder
		publicKey string
	)
	if publicKey, err = d.parseBase64KeyToHex(i.PublicKey); err != nil {
		return
	}
	sb.WriteString("public_key=" + publicKey + "\n")
	sb.WriteString("endpoint=" + i.Endpoint + "\n")
	sb.WriteString("persistent_keepalive_interval=0\n")
	sb.WriteString("preshared_key=0000000000000000000000000000000000000000000000000000000000000000\n")
	sb.WriteString("allowed_ip=0.0.0.0/0\n")
	sb.WriteString("allowed_ip=::0/0\n")
	str = sb.String()
	return
}

func (d *Wireguard) createLogger() *device.Logger {
	return &device.Logger{
		Verbosef: func(format string, args ...any) {
			log.Debugf(format, args)
		},
		Errorf: func(format string, args ...any) {
			log.Warnf(format, args)
		},
	}
}

func (d *Wireguard) Dial(network, address string) (net.Conn, error) {
	if !d.Ready() {
		return nil, io.ErrNoProgress
	}
	return d.netstack.Dial(network, address)
}

func (d *Wireguard) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if !d.Ready() {
		return nil, io.ErrNoProgress
	}
	return d.netstack.DialContext(ctx, network, address)
}

func (d *Wireguard) Mount(ctx context.Context) (err error) {
	var (
		tunDevice tun.Device
		address   []netip.Addr
		dns       []netip.Addr
		mtu       uint
		uapi      string
	)
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if address, dns, mtu, err = d.parseInterface(d.Config.Interface); err != nil {
		return
	}
	if tunDevice, d.netstack, err = netstack.CreateNetTUN(address, dns, int(mtu)); err != nil {
		return
	}
	d.device = device.NewDevice(tunDevice, conn.NewDefaultBind(), d.createLogger())
	if uapi, err = d.buildPeer(d.Config.Peer); err != nil {
		return
	}
	log.Debugf("connecting peer \n %s", uapi)
	if err = d.device.IpcSet(uapi); err != nil {
		return
	}
	if err = d.device.Up(); err != nil {
		return
	}
	atomic.StoreInt32(&d.readyFlag, 1)
	return
}

func (d *Wireguard) Umount() (err error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if atomic.CompareAndSwapInt32(&d.readyFlag, 1, 0) {
		if err = d.device.Down(); err != nil {

		}
	}
	return
}

func (d *Wireguard) Reload() (err error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if err = d.device.Down(); err == nil {
		err = d.device.Up()
	}
	return
}

func newWireguard(cfg config.Wireguard) *Wireguard {
	return &Wireguard{Config: cfg}
}

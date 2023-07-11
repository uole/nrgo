package nrgo

import (
	"github.com/uole/nrgo/pkg/multiplex"
	"github.com/uole/nrgo/pkg/packet"
	"io"
	"time"
)

type (
	Handshake func(*packet.Frame, multiplex.Stream) (io.ReadWriteCloser, error)

	remoteConfig struct {
		Version string `json:"version"`
		Domain  string `json:"domain"`
	}

	Address struct {
		Client string `json:"client,omitempty"`
		Tunnel string `json:"tunnel,omitempty"`
	}

	ServeInfo struct {
		ID        string      `json:"id"`
		Proto     string      `json:"proto"`
		Address   Address     `json:"address"`
		ConnSize  int         `json:"conn_size"`
		SecretKey string      `json:"secretKey"`
		Slaves    []SlaveInfo `json:"slaves"`
		Uptime    time.Time   `json:"uptime"`
	}

	SlaveInfo struct {
		ID       string    `json:"id"`
		OS       string    `json:"os"`
		Name     string    `json:"name"`
		State    string    `json:"state"`
		Proto    string    `yaml:"proto"`
		Address  Address   `json:"address"`
		ConnSize int       `json:"conn_size"`
		Uptime   time.Time `json:"uptime"`
		PingTime time.Time `json:"ping_time"`
	}

	ConnectionInfo struct {
		Proto     string  `json:"proto"`
		ConnSize  int     `json:"conn_size"`
		Address   Address `json:"address"`
		SecretKey []byte  `json:"secret_key"`
	}

	NodeInfo struct {
		ID      string    `json:"id"`
		Name    string    `json:"name"`
		Country string    `json:"country"`
		OS      string    `json:"os"`
		IP      string    `json:"ip"`
		CPU     int       `json:"cpu"`
		Uptime  time.Time `json:"uptime"`
	}

	GeoInfo struct {
		Organization    string  `json:"organization"`
		Longitude       float64 `json:"longitude"`
		City            string  `json:"city"`
		Timezone        string  `json:"timezone"`
		Isp             string  `json:"isp"`
		Offset          int     `json:"offset"`
		Region          string  `json:"region"`
		Asn             int     `json:"asn"`
		AsnOrganization string  `json:"asn_organization"`
		Country         string  `json:"country"`
		IP              string  `json:"ip"`
		Latitude        float64 `json:"latitude"`
		PostalCode      string  `json:"postal_code"`
		ContinentCode   string  `json:"continent_code"`
		CountryCode     string  `json:"country_code"`
		RegionCode      string  `json:"region_code"`
	}

	serveInfoResponse struct {
		Code   int       `json:"code"`
		Reason string    `json:"reason"`
		Data   ServeInfo `json:"data"`
	}
)

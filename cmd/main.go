package main

import (
	"flag"
	"fmt"
	"git.nspix.com/golang/kos"
	"github.com/uole/nrgo"
	"github.com/uole/nrgo/config"
	"github.com/uole/nrgo/version"
	yaml "gopkg.in/yaml.v3"
	"os"
)

var (
	serviceFlag = flag.Bool("service", false, "Print service template")
	configFlag  = flag.String("config", "", "Config file name")
)

func printService() {
	fmt.Println(`
[Unit]
Description= Network provider client

[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart=/usr/local/bin/nrgo
Restart=always
RestartSec=60

[Install]
WantedBy=multi-user.target
`)
}

func main() {
	var (
		err error
		buf []byte
	)
	flag.Parse()
	if *serviceFlag {
		printService()
		os.Exit(0)
	}
	cfg := config.New()
	if *configFlag != "" {
		if buf, err = os.ReadFile(*configFlag); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err = yaml.Unmarshal(buf, cfg); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
	svr := kos.Init(
		kos.WithName("github.com/uole/nrgo", version.Version),
		kos.WithServer(nrgo.New(cfg)),
		kos.WithDirectHttp(),
	)
	if err = svr.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

package main

import (
	"flag"
	"fmt"
	"git.nspix.com/golang/kos"
	"github.com/uole/nrgo"
	"github.com/uole/nrgo/version"
	"os"
)

var (
	serviceFlag = flag.Bool("service", false, "Print service template")
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
	flag.Parse()
	if *serviceFlag {
		printService()
		os.Exit(0)
	}
	svr := kos.Init(
		kos.WithName("github.com/uole/nrgo", version.Version),
		kos.WithServer(nrgo.New()),
	)
	if err := svr.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

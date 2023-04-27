package main

import (
	"flag"
	"fmt"
	"git.nspix.com/golang/kos"
	"github.com/uole/nrgo"
	"github.com/uole/nrgo/version"
	"os"
)

func main() {
	flag.Parse()
	svr := kos.Init(
		kos.WithName("github.com/uole/nrgo", version.Version),
		kos.WithServer(nrgo.New()),
	)
	if err := svr.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

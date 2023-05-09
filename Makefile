GOPATH:=$(shell go env GOPATH)
DATETIME:=$(shell date "+%Y-%m-%d %H:%M:%S")
VERSION:="0.0.1"
PACKAGENAME:="github.com/uole/nrgo"

.PHONY: build
build:
	go mod tidy
	go mod vendor
	CGO_ENABLED=0 go build -ldflags "-s -w -X '$(PACKAGENAME)/version.Version=$(VERSION)' -X '$(PACKAGENAME)/version.BuildDate=$(DATETIME)'" -o ./bin/nrgo ./cmd/main.go

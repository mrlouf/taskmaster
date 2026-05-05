#!/bin/bash

# go version > 1.25
go install github.com/air-verse/air@latest

export GOPATH=$HOME/xxxxx
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin
export PATH=$PATH:$(go env GOPATH)/bin
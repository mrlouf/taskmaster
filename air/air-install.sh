#!/bin/bash

# go version > 1.25
go install github.com/air-verse/air@latest

export PATH=$PATH:$(go env GOPATH)/bin
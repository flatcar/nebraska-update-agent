#!/bin/bash

go build -o luc updater/main.go
go build -o nebraska-agent agent/main.go
docker build -t quay.io/kinvolk/luc .

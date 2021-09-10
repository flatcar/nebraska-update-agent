#!/bin/bash

go build -o luc main.go
docker build -t quay.io/kinvolk/luc .

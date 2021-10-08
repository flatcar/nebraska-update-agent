#!/bin/bash

env GOOS=linux GOARCH=amd64 go build -o nuc main.go
docker buildx build --load -t quay.io/kinvolk/nuc --platform linux/amd64 .

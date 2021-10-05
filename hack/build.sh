#!/bin/bash

env GOOS=linux GOARCH=amd64 go build -o luc main.go
docker buildx build --load -t quay.io/kinvolk/luc --platform linux/amd64 .
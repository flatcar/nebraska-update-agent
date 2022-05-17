#!/bin/bash

env GOOS=linux GOARCH=amd64 go build -o nua main.go
docker buildx build --load -t quay.io/kinvolk/nua --platform linux/amd64 .

#/usr/bin/env bash

export ZMX_SESSION_PREFIX="${ZMX_SESSION_PREFIX:-pgit.}"
zmx run fmt test -z "$(gofmt -l .)"
zmx run lint -d golangci-lint run
zmx run test -d go test -race ./...
zmx wait '*'
printf 'success!\n'

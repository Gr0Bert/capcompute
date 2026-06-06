#!/bin/sh
set -eu

cd "$(dirname "$0")"

: "${XDG_CACHE_HOME:=/tmp/capcompute-tinygo-cache}"
export XDG_CACHE_HOME

tinygo build -target wasip1 -buildmode=c-shared -tags tinygo -o ../agent.wasm agent.go

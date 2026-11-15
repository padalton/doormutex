#!/bin/sh
# Elgato's app on macOS starts CodePathMac; pick the binary for this Mac.
dir=$(dirname "$0")
case $(uname -m) in
arm64) exec "$dir/doormutex-streamdeck-darwin-arm64" "$@" ;;
*) exec "$dir/doormutex-streamdeck-darwin-amd64" "$@" ;;
esac

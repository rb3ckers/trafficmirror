#!/bin/sh

# Takes these environment variables:
#
# LISTEN_PORT: port to listen on (defaults to 8080)
# MAIN: reverse proxy to this address (defaults to localhost:8888)
# USERNAME & PASSWORD: if USERNAME is set protect targets endpoint with basic auth (default to empty)

extraParams="${1}"
listenPort="${LISTEN_PORT:-8080}"
main="${MAIN:-localhost:8888}"

if [ -n "${USERNAME}" ]; then
  echo "${USERNAME}:${PASSWORD}" > /password.file
  extraParams="${extraParams} -password /password.file"
fi

cmd="/trafficmirror -listen ":${listenPort}" -main=${main} ${extraParams}"

exec "${cmd}"

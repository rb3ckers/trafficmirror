#!/bin/sh

[ -n "${TRACE+x}" ] && set -x

set -e

# Takes these environment variables:
#
# LISTEN_PORT: port to listen on (defaults to 8080)
# MAIN: reverse proxy to this address (defaults to localhost:8888)
# USERNAME & PASSWORD: if USERNAME is set protect targets endpoint with basic auth (default to empty)

extraParams="${1}"
listenPort="${LISTEN_PORT:-8080}"
main="${MAIN:-localhost:8888}"
passwordFile="${PASSWORD_FILE:-/tmp/password.file}"

if [ -n "${USERNAME}" ]; then
  echo "${USERNAME}:${PASSWORD}" > "${passwordFile}"
  extraParams="${extraParams} -password ${passwordFile}"
fi

/trafficmirror -listen ":${listenPort}" -main="${main}" "${extraParams}"

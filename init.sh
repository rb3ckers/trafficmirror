#!/bin/sh

# Takes these environment variables:
#
# LISTEN_PORT: port to listen on (defaults to 8080)
# MAIN: reverse proxy to this address (defaults to localhost:8888)
# USERNAME & PASSWORD: if USERNAME is set protect targets endpoint with basic auth (default to empty)

listen_port=${LISTEN_PORT:-8080}
main=${MAIN:-localhost:8888}
extra_params=$1

if [ ! -z "${USERNAME}" ]
then
  printf "${USERNAME}:${PASSWORD}" > /password.file
  extra_params="${extra_params} -password /password.file"
fi

cmd="/trafficmirror -listen ":${listen_port}" -main=${main} $extra_params"

echo "$cmd"
eval "$cmd"

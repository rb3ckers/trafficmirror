#!/bin/sh

LISTEN_PORT=${LISTEN_PORT:-7077}
MAIN=${MAIN:-localhost:8888}
EXTRA_PARAMS=$1

echo /trafficmirror /trafficmirror -listen ":${LISTEN_PORT}" -main=${MAIN} $EXTRA_PARAMS
/trafficmirror -listen ":${LISTEN_PORT}" -main=${MAIN} $EXTRA_PARAMS

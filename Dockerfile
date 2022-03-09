###########
# Builder #
###########

FROM golang:1.17-alpine AS builder

RUN apk add --no-cache git

COPY . /build

WORKDIR /build

RUN set -ex \
    && GOOS=linux GOARCH=amd64 go build -o /build/trafficmirror

RUN /build/trafficmirror --help

#######
# App #
#######

FROM alpine:latest AS app

ENV PERSISTENT_PACKAGES="ca-certificates tini"

# Copy support files
COPY rootfs /

# Upgrade OS packages for security
RUN apk upgrade --no-cache --available \
    && apk add --no-cache ${PERSISTENT_PACKAGES}

# Copy artifacts from builder container
COPY --from=builder /build/trafficmirror /trafficmirror

# Switch to non-root user
USER nobody

EXPOSE 8080

ENTRYPOINT ["/sbin/tini", "--"]

CMD ["/docker-entrypoint.sh"]

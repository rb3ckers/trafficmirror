###########
# Builder #
###########

FROM golang:1.11-alpine AS builder

RUN apk add --no-cache git

COPY . $GOPATH/src/github.com/rb3ckers/trafficmirror

WORKDIR $GOPATH/src/github.com/rb3ckers/trafficmirror

RUN set -ex \
    && go get -u -v github.com/golang/dep/cmd/dep \
    && dep ensure -v \
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

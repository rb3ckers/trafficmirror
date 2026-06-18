###########
# Builder #
###########

FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH

RUN apk add --no-cache git

COPY . /build

WORKDIR /build

RUN set -ex \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -o /build/trafficmirror

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

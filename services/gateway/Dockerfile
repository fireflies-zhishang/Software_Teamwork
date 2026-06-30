ARG IMAGE_REGISTRY_PREFIX=
ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.22

FROM ${IMAGE_REGISTRY_PREFIX}golang:${GO_VERSION}-alpine AS build

WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org
ARG ALPINE_MIRROR=
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}
RUN if [ -n "$ALPINE_MIRROR" ]; then \
        sed -i "s|https://dl-cdn.alpinelinux.org/alpine|$ALPINE_MIRROR|g" /etc/apk/repositories; \
    fi
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/server

FROM ${IMAGE_REGISTRY_PREFIX}alpine:${ALPINE_VERSION}

ARG ALPINE_MIRROR=
RUN --mount=type=cache,target=/var/cache/apk \
    if [ -n "$ALPINE_MIRROR" ]; then \
        sed -i "s|https://dl-cdn.alpinelinux.org/alpine|$ALPINE_MIRROR|g" /etc/apk/repositories; \
    fi \
    && apk add --update-cache --cache-dir /var/cache/apk ca-certificates

RUN addgroup -S gateway && adduser -S -G gateway gateway
COPY --from=build /out/gateway /usr/local/bin/gateway

USER gateway
EXPOSE 8080

ENTRYPOINT ["gateway"]

# syntax=docker/dockerfile:1
# Build from the repository root: docker build -f nas/Dockerfile .
ARG GO_VERSION=1.27

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
ARG VERSION=dev
WORKDIR /src
COPY core/Clash.Meta/go.mod core/Clash.Meta/go.sum core/Clash.Meta/
COPY nas/go.mod nas/go.sum nas/
RUN cd nas && go mod download
COPY core/Clash.Meta core/Clash.Meta
COPY nas nas
RUN cd nas && \
    GOARM="${TARGETVARIANT#v}" CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -tags with_gvisor \
      -ldflags "-s -w -X main.appVersion=${VERSION}" \
      -o /out/flclash-nas .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/flclash-nas /usr/local/bin/flclash-nas
ENV FLCLASH_DATA=/data FLCLASH_UI_ADDR=:8080 TZ=Asia/Shanghai
VOLUME /data
EXPOSE 8080 7890
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD wget -q -O /dev/null http://127.0.0.1:8080/ || exit 1
ENTRYPOINT ["/usr/local/bin/flclash-nas"]

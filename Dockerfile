FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
ARG GOPROXY=https://goproxy.cn,direct
RUN go env -w GOPROXY="$GOPROXY" && go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/ ./web/
ARG TARGETOS
ARG TARGETARCH
ARG APP_VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$APP_VERSION" -o /out/clash-config-manager ./cmd/app

FROM --platform=$BUILDPLATFORM alpine:3.22 AS core
RUN apk add --no-cache ca-certificates curl
ARG TARGETARCH
ARG MIHOMO_VERSION=v1.19.33
RUN case "$TARGETARCH" in \
      amd64) asset="mihomo-linux-amd64-compatible-${MIHOMO_VERSION}.gz" ;; \
      arm64) asset="mihomo-linux-arm64-${MIHOMO_VERSION}.gz" ;; \
      *) echo "Unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac \
    && mkdir /out \
    && curl --fail --location --retry 3 "https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VERSION}/${asset}" -o /tmp/mihomo.gz \
    && gzip -dc /tmp/mihomo.gz > /out/mihomo \
    && chmod 755 /out/mihomo \
    && curl --fail --location --retry 3 "https://raw.githubusercontent.com/MetaCubeX/mihomo/${MIHOMO_VERSION}/LICENSE" -o /out/LICENSE \
    && printf 'Mihomo %s\nSource: https://github.com/MetaCubeX/mihomo/archive/refs/tags/%s.tar.gz\n' "$MIHOMO_VERSION" "$MIHOMO_VERSION" > /out/SOURCE.txt

FROM alpine:3.22
LABEL org.opencontainers.image.source="https://github.com/mrlihx/clash-config-manager"
RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && addgroup -S -g 10001 app \
    && adduser -S -u 10001 -G app app \
    && mkdir -p /app/data \
    && chown -R app:app /app
COPY --from=build /out/clash-config-manager /usr/local/bin/clash-config-manager
COPY --from=core /out/mihomo /usr/local/bin/mihomo
COPY --from=core /out/LICENSE /out/SOURCE.txt /usr/share/licenses/mihomo/
COPY LICENSE /usr/share/licenses/clash-config-manager/LICENSE
COPY THIRD_PARTY_NOTICES.md /usr/share/licenses/clash-config-manager/THIRD_PARTY_NOTICES.md
COPY deploy/entrypoint.sh /usr/local/bin/manager-entrypoint
WORKDIR /app
USER app
EXPOSE 8787
ENTRYPOINT ["/bin/sh", "/usr/local/bin/manager-entrypoint", "-config", "/app/config.yaml"]

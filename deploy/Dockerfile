FROM debian:trixie-20261104-slim AS css
ARG TARGETARCH
ARG TAILWIND_VERSION=v4.3.5
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*
RUN case "${TARGETARCH:-amd64}" in amd64) arch=x64 ;; arm64) arch=arm64 ;; *) echo "unsupported arch ${TARGETARCH}" && exit 1 ;; esac \
 && curl -fsSL -o /usr/local/bin/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${arch}" \
 && chmod +x /usr/local/bin/tailwindcss
WORKDIR /src
COPY internal/web ./internal/web
RUN tailwindcss -i internal/web/static/css/input.css -o internal/web/static/css/app.css --minify

FROM golang:1.27.4-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=css /src/internal/web/static/css/app.css ./internal/web/static/css/app.css
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/opsmanager ./cmd/opsmanager

FROM debian:trixie-20261104-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --system --gid 10001 ops \
 && useradd --system --uid 10001 --gid ops --no-create-home --shell /usr/sbin/nologin ops \
 && mkdir -p /data/uploads \
 && chown -R ops:ops /data
COPY --from=build /out/opsmanager /usr/local/bin/opsmanager
USER ops:ops
ENV OPS_UPLOADS_DIR=/data/uploads
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD ["opsmanager", "healthcheck"]
ENTRYPOINT ["opsmanager"]
CMD ["serve"]

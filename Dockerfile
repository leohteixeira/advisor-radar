# syntax=docker/dockerfile:1
FROM golang:1.26.5-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN set -eu; for service in account-sim advisory triage cases timeline-indexer bff db; do \
      CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/$service ./cmd/$service; \
    done

FROM debian:bookworm-slim AS backend
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl netcat-openbsd \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=go-build /out/ ./bin/
COPY go.mod ./go.mod
COPY migrations/ ./migrations/
COPY seeds/ ./seeds/
USER nobody
CMD ["/app/bin/bff"]

FROM node:24.19.0-bookworm-slim AS web-build
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@11.22.0 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM caddy:2.10.2-alpine AS web
COPY deploy/Caddyfile.production /etc/caddy/Caddyfile
COPY --from=web-build /src/web/dist /srv
USER nobody
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=12 CMD wget -q -O /dev/null http://127.0.0.1:8080/

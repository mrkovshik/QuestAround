# Сборка в три шага: фронтенд (Node) → бинарник с вшитым фронтендом (Go) → маленький образ.

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
# vite кладёт сборку в ../internal/webui/dist/app
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /src
# Если proxy.golang.org недоступен — модули берутся с зеркала или напрямую из GitHub.
ENV GOPROXY=https://proxy.golang.org,https://goproxy.io,direct \
    CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist/app ./internal/webui/dist/app
RUN go build -trimpath -ldflags="-s -w" -o /out/questline ./cmd/questline

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 app \
 && mkdir -p /data/media && chown -R app /data
COPY --from=build /out/questline /usr/local/bin/questline
USER app
ENV QUESTLINE_ADDR=:8080 \
    QUESTLINE_MEDIA_DIR=/data/media \
    TZ=Europe/Moscow
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["questline"]
CMD ["serve"]

FROM node:24-bookworm-slim AS frontend
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-bookworm AS backend
WORKDIR /src
ARG VERSION=dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /arkiv ./cmd/gallery

FROM backend AS verify
RUN apt-get update && apt-get install -y --no-install-recommends libvips-tools ffmpeg libimage-exiftool-perl
RUN ARKIV_INTEGRATION=1 go test ./...

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends libvips-tools ffmpeg libimage-exiftool-perl ca-certificates tzdata && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 10001 arkiv && useradd -u 10001 -g arkiv -M arkiv \
    && mkdir /data /cache && chown arkiv:arkiv /data /cache
COPY --from=backend /arkiv /usr/local/bin/arkiv
RUN ln -s /usr/local/bin/arkiv /usr/local/bin/gallery
USER 10001:10001
# Application defaults live in internal/config.
ENV VIPS_CONCURRENCY=1
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD ["arkiv", "healthcheck"]
ENTRYPOINT ["arkiv"]

# Nightly backups (hp backup -every 24h): the admin tool next to PostgreSQL's
# own pg_dump and pg_restore, of the same major version as the database, and
# rclone for the copies off the server.
FROM golang:1.27 AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hp ./cmd/hp

# rclone copies each backup off the server (HP_OFFSITE_*).
FROM rclone/rclone:1.68.2 AS rclone

FROM postgres:16
COPY --from=build /out/hp /app/hp
COPY --from=rclone /usr/local/bin/rclone /usr/local/bin/rclone
CMD ["/app/hp", "backup", "-every", "24h"]

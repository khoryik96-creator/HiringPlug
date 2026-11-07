# Nightly backups (hp backup -every 24h): the admin tool next to PostgreSQL's
# own pg_dump and pg_restore, of the same major version as the database.
FROM golang:1.27 AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hp ./cmd/hp

FROM postgres:16
COPY --from=build /out/hp /app/hp
CMD ["/app/hp", "backup", "-every", "24h"]

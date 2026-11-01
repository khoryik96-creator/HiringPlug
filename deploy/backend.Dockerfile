# The Go API, worker and admin tool (hp) in one small image.
FROM golang:1.27 AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/api ./cmd/worker ./cmd/hp
# Uploaded files live here; owned by the non-root user so a fresh volume is writable.
RUN mkdir -p /data/files

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /app/
COPY --from=build --chown=65532:65532 /data /data
WORKDIR /app
USER nonroot
EXPOSE 8080
CMD ["/app/api"]

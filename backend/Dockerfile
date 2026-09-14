# --- Build stage ---
# Compiles the Go binary inside a container that already has the Go
# toolchain, so you don't need Go installed on your own machine.
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY . .
# go mod tidy resolves and downloads the SQLite driver (modernc.org/sqlite,
# pure Go - no cgo/C compiler needed) and writes go.sum, since neither is
# committed to source (this needs network access, which the Docker build
# environment has). It's a pure-Go driver, so no gcc/musl-dev is required
# even on this alpine base image.
RUN go mod tidy
RUN go build -o server .

# --- Run stage ---
# Copies only the compiled binary into a tiny final image.
FROM alpine:3.20
WORKDIR /app
COPY --from=build /app/server .
# Data directory for the SQLite database file; mount a volume here to
# persist playlists/sync state across container restarts (see docker-compose.yml).
RUN mkdir -p /app/data
ENV PORT=8080
ENV DATA_FILE=/app/data/store.db
ENV FRONTEND_ORIGIN=*
EXPOSE 8080
CMD ["./server"]

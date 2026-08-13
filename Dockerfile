# Multi-stage Docker build for Supermicro License Generator & SUM Activator
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
COPY upstream ./upstream
COPY main.go ./
COPY static ./static

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o supermicro-license-generator main.go

# Debian slim runtime for full Linux glibc compatibility with the Supermicro
# SUM binary (which is not statically linked).
FROM debian:bookworm-slim

# ca-certificates is required for the Redfish/IPMI Web HTTPS calls the app makes.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/supermicro-license-generator .
COPY --from=builder /app/static ./static

EXPOSE 8080

ENV PORT=8080
# Publish on all interfaces inside the container so the mapped port is reachable.
ENV HOST=0.0.0.0
# Headless container: do not attempt to launch a browser.
ENV NO_BROWSER=1
# Persistent data dir for a SUM toolchain uploaded through the web UI. Kept on a
# volume so the installed SUM survives image rebuilds/updates.
ENV SUM_DATA_DIR=/app/data
VOLUME ["/app/data"]

# Key generation, decoding and MAC reading (Redfish/IPMI Web) work out of the
# box. Direct SUM activation additionally needs the proprietary SUM binary,
# which is NOT bundled (it is not redistributable). Provide it either by:
#   1. uploading a SUM .zip/.tar.gz in the web UI (persisted in /app/data), or
#   2. mounting it and pointing SUM_PATH at it, e.g.:
#        docker run -p 127.0.0.1:8080:8080 \
#          -e SUM_PATH=/app/sum_tool/sum \
#          -v /opt/sum_2.15.0_Linux_x86_64:/app/sum_tool:ro \
#          ghcr.io/ttolyanich/supermicro-license-generator:latest

CMD ["./supermicro-license-generator"]

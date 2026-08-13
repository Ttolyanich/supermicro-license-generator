# Multi-stage Docker build for Supermicro License Generator & SUM Activator
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY upstream ./upstream
COPY main.go ./
COPY static ./static

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o supermicro-license-generator main.go

# Debian slim runtime for full Linux glibc compatibility with Supermicro SUM binary
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    unzip \
    procps \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/supermicro-license-generator .
COPY --from=builder /app/static ./static

# Download SUM 2.15 tool from Google Drive mirror using gdown with --break-system-packages
RUN pip install --no-cache-dir --break-system-packages gdown && \
    gdown "1Vx3SUKApd5q-G7-RvHuioPPddTTBwpli" -O /tmp/sum.zip --fuzzy && \
    mkdir -p /tmp/sum_out && \
    unzip -q /tmp/sum.zip -d /tmp/sum_out && \
    mkdir -p /app/sum_tool && \
    cp -r /tmp/sum_out/*/* /app/sum_tool/ && \
    rm -rf /tmp/sum* && \
    chmod +x /app/sum_tool/sum && \
    ln -s /app/sum_tool/sum /app/sum

EXPOSE 8080

ENV PORT=8080
ENV SUM_PATH=/app/sum

CMD ["./supermicro-license-generator"]

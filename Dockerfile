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
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/supermicro-license-generator .
COPY --from=builder /app/static ./static

# Optional SUM download (resilient, does not fail build if network is restricted)
RUN mkdir -p /app/sum_tool && \
    (curl -sSL "https://drive.google.com/uc?export=download&id=1Vx3SUKApd5q-G7-RvHuioPPddTTBwpli&confirm=t" -o /tmp/sum.zip || true) && \
    (unzip -q /tmp/sum.zip -d /tmp/sum_out 2>/dev/null || true) && \
    (cp -r /tmp/sum_out/*/* /app/sum_tool/ 2>/dev/null || true) && \
    (chmod +x /app/sum_tool/sum 2>/dev/null || true) && \
    (ln -s /app/sum_tool/sum /app/sum 2>/dev/null || true) && \
    rm -rf /tmp/sum*

EXPOSE 8080

ENV PORT=8080
ENV SUM_PATH=/app/sum

CMD ["./supermicro-license-generator"]

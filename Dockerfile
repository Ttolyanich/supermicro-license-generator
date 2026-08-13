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
    procps \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/supermicro-license-generator .
COPY --from=builder /app/static ./static

# Copy SUM tool files
COPY sum_2.15.0_Linux_x86_64_20251104/sum_2.15.0_Linux_x86_64/ /app/sum_tool/

# Make sum binary executable and symlink to /app/sum
RUN chmod +x /app/sum_tool/sum && ln -s /app/sum_tool/sum /app/sum

EXPOSE 8080

ENV PORT=8080
ENV SUM_PATH=/app/sum

CMD ["./supermicro-license-generator"]

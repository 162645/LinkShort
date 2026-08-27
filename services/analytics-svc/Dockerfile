# Stage 1: Build the application
FROM golang:1.23-alpine AS builder

# 修改处 1: 设置 Go 代理，防止 go mod download 失败
ENV GOPROXY=https://goproxy.cn,direct

# Set working directory
WORKDIR /app

# 修改处 2: 在 apk update 前替换为阿里云镜像站，解决系统插件下载失败
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && apk add --no-cache git ca-certificates tzdata

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project (needed for proto and shared utilities)
COPY . .

# Build the analytics service
WORKDIR /app/services/analytics-svc
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o analytics-svc ./cmd/

# Stage 2: Create the final image
FROM alpine:3.19

# Install runtime dependencies with updated repositories
# 修改处 3: 在第二阶段也要换源，因为这里也用到了 apk add
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && apk add --no-cache ca-certificates tzdata netcat-openbsd

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/services/analytics-svc/analytics-svc .

# Copy any config files if needed
# COPY --from=builder /app/services/analytics-svc/config/ ./config/

# Change ownership of the app directory
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose ports (gRPC port and Prometheus metrics port)
EXPOSE 50052 8002

# Health check for gRPC service
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 50052 || exit 1

# Run the binary
ENTRYPOINT ["./analytics-svc"] 
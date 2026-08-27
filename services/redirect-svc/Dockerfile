# Stage 1: Build the application
FROM golang:1.23-alpine AS builder

# --- 修改处 1: 设置 Go 环境变量加速依赖下载 ---
ENV GOPROXY=https://goproxy.cn,direct

# Set working directory
WORKDIR /app

# Install build dependencies with updated repositories
# --- 修改处 2: 替换 Alpine 官方源为阿里云镜像站 ---
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && apk add --no-cache git ca-certificates tzdata

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project (needed for proto and shared utilities)
COPY . .

# Build the redirect service
WORKDIR /app/services/redirect-svc
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o redirect-svc ./cmd/

# Stage 2: Create the final image
FROM alpine:3.19

# --- 修改处 3: 运行环境阶段同样需要替换阿里云镜像站 ---
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && apk add --no-cache ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/services/redirect-svc/redirect-svc .

# Copy any config files if needed
# COPY --from=builder /app/services/redirect-svc/config/ ./config/

# Change ownership of the app directory
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose ports (gRPC port and Prometheus metrics port)
EXPOSE 50053 9091

# Health check for gRPC service (proxy check since it's gRPC)
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9091/metrics || exit 1

# Run the binary
ENTRYPOINT ["./redirect-svc"] 
# 第一阶段：编译构建环境
FROM golang:1.23-alpine AS builder

# --- 修改处 1: 设置 Go 环境变量加速，解决 go mod download 卡死 ---
ENV GOPROXY=https://goproxy.cn,direct

# 设置构建工作目录
WORKDIR /app

# 安装必要的系统工具和时区插件
# --- 修改处 2: 替换 Alpine 官方源为阿里云镜像站，解决 apk update 报错 ---
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && apk add --no-cache git ca-certificates tzdata

# 利用缓存机制：先处理依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 拷贝全量源码以供编译使用
COPY . .

# 切换到具体的微服务源码目录
WORKDIR /app/services/rest-api-svc
# 执行静态编译，生成无依赖的二进制文件
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o rest-api-svc ./cmd/

# 第二阶段：生产运行环境
FROM alpine:3.19

# 安装运行时必需的证书与检查工具
# --- 修改处 3: 运行环境阶段同样需要替换阿里云镜像站 ---
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && apk add --no-cache ca-certificates tzdata wget

# 建立普通用户组及用户以增强安全性
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# 设置程序运行目录
WORKDIR /app

# 从构建阶段拷贝编译好的二进制成品
COPY --from=builder /app/services/rest-api-svc/rest-api-svc .

# 此处预留配置文件拷贝指令
# COPY --from=builder /app/services/rest-api-svc/config/ ./config/

# 将目录所有权移交给普通用户
RUN chown -R appuser:appgroup /app

# 切换到安全受限用户执行
USER appuser

# 开启服务通信端口
EXPOSE 8080

# 配置服务存活自动检测机制
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# 容器启动时默认运行程序，，“一旦这个容器跑起来了，请立刻运行目录下这个叫 rest-api-svc 的二进制文件”。
ENTRYPOINT ["./rest-api-svc"]
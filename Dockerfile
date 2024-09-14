# 使用官方 Golang 镜像作为构建阶段的基础镜像
FROM golang:1.22 as builder

# 设置工作目录
WORKDIR /app

# 将 Go 项目的所有文件复制到工作目录中
COPY .. .

ENV GOPROXY=https://goproxy.cn,direct
# 构建 Go 二进制文件
RUN go mod tidy
RUN go build -o exporter .

# 使用一个更小的镜像作为运行阶段的基础镜像
FROM alpine:3.18

# 安装 MySQL 客户端依赖
RUN apk add --no-cache mysql-client

# 设置工作目录
WORKDIR /app

# 从构建阶段复制生成的二进制文件到当前镜像中
COPY --from=builder /app/mysql_info_exporter /app/mysql_info_exporter
COPY config.yaml /app/config.yaml

# 暴露 Prometheus 的端口
EXPOSE 18080

# 设置容器启动时的默认命令
CMD ["/app/mysql_info_exporter"]

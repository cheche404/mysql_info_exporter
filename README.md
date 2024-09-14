# exporter
## mysql_info_exporter
查询 MySQL 表所占空间的大小、以及每个库的连接数；将查询结果封装为 Prometheus 指标。

### 四个指标
```text
- mysql_table_size_bytes  Size of tables in MySQL, in bytes.
- mysql_index_size_bytes  Size of indexes in MySQL, in bytes.
- mysql_table_rows        Number of rows in MySQL tables.
- mysql_processlist_count Number of processes in the processlist, grouped by user and database.
```

### 查询语句
```sql
-- 查询表所占空间、表行数
SELECT table_schema AS ` + "`db_name`" + `, table_name AS ` + "`table`" + `, table_rows,
data_length AS ` + "`data_size_bytes`" + `, index_length AS ` + "`index_size_bytes`" + `
FROM information_schema.tables
ORDER BY data_length DESC, index_length DESC);
-- 查询连接数
SHOW PROCESSLIST;
```

### 编译方式
```shell
go env -w GOOS=linux
go env -w GOARCH=amd64
go build -o exporter main.go
```

### Dockerfile

```dockerfile
# 使用官方 Golang 镜像作为构建阶段的基础镜像
FROM golang:1.22 as builder
# 设置工作目录
WORKDIR /app
# 将 Go 项目的所有文件复制到工作目录中
COPY .. .
ENV GOPROXY=https://goproxy.cn,direct
# 构建 Go 二进制文件
RUN go mod tidy
RUN go build -o mysql_info_exporter .
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
```

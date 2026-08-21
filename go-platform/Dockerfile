FROM golang:1.25-alpine AS builder

WORKDIR /app

# 复制所有文件（包括 vendor 目录）
COPY . .

# 使用 vendor 模式构建，无需网络下载依赖
RUN CGO_ENABLED=0 go build -mod=vendor -o server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/server .
COPY migrations/ ./migrations/

EXPOSE 8080
CMD ["./server"]

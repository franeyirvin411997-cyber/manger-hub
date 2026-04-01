FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build Manager
RUN CGO_ENABLED=0 GOOS=linux go build -o /manager ./cmd/manager
# Build Node
RUN CGO_ENABLED=0 GOOS=linux go build -o /node ./cmd/node

# ----- Runtime -----
FROM alpine:latest

WORKDIR /app
COPY --from=builder /manager /usr/local/bin/manager
COPY --from=builder /node /usr/local/bin/node

# 安装执行环境所需依赖：docker 客户端 (供 node 调用)
RUN apk add --no-cache docker-cli

EXPOSE 8080 50051

CMD ["/usr/local/bin/manager"]

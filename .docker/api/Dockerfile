# .docker/api/Dockerfile
# API 服務多階段建構

FROM golang:1.23-alpine AS builder

WORKDIR /app

# 設定 Alpine 鏡像（解決網路問題）並安裝必要工具
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories || true && \
    apk update && \
    apk add --no-cache git ca-certificates tzdata

# 複製 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 複製原始碼
COPY . .

# 編譯 API
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /api ./cmd/api

# ============================================
# 最終映像
# ============================================
FROM alpine:3.19

# 設定 Alpine 鏡像並安裝所有必要套件（合併 RUN 減少 layer）
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories || true && \
    apk update && \
    apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

COPY --from=builder /api .

EXPOSE 8080

CMD ["./api"]

# .docker/worker/Dockerfile
# Worker 服務多階段建構
# 使用 Ubuntu 24.04 LTS 作為基底映像

# ============================================
# Builder Stage
# ============================================
FROM golang:1.23-bookworm AS builder

WORKDIR /app

# 複製 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 複製原始碼
COPY . .

# 編譯 Worker（靜態連結）
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /worker ./cmd/worker

# ============================================
# 最終映像 - Ubuntu 24.04 LTS (最小化)
# ============================================
FROM ubuntu:24.04

# 設定環境變數避免互動式安裝提示
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Taipei

# 安裝必要套件並清理快取（減少映像大小）
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

WORKDIR /app

COPY --from=builder /worker .

CMD ["./worker"]

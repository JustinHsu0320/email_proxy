# .docker/api/Dockerfile
# API 服務多階段建構
# 使用 scratch (空白映像) - 無需網路下載任何套件

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

# 編譯 API（完全靜態連結）
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o /api ./cmd/api

# 從 builder 複製 CA 憑證和時區資料
RUN cp /etc/ssl/certs/ca-certificates.crt /ca-certificates.crt
RUN cp -r /usr/share/zoneinfo /zoneinfo

# ============================================
# 最終映像 - scratch (空白映像，約 0 bytes)
# 不需要任何網路連線，不需要安裝任何套件
# ============================================
FROM scratch

# 設定時區
ENV TZ=Asia/Taipei

# 複製 CA 憑證（HTTPS 連線需要）
COPY --from=builder /ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# 複製時區資料
COPY --from=builder /zoneinfo /usr/share/zoneinfo

# 複製編譯好的二進制檔
COPY --from=builder /api /api

EXPOSE 8080

ENTRYPOINT ["/api"]

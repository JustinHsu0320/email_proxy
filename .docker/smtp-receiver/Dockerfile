# .docker/smtp-receiver/Dockerfile
# SMTP Inbound Server 多階段建構

FROM golang:1.23-alpine AS builder

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY
WORKDIR /app

# 安裝必要工具
RUN apk add --no-cache git ca-certificates tzdata

# 複製 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 複製原始碼
COPY . .

# 編譯 SMTP Receiver
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /smtp-receiver ./cmd/smtp-receiver

# ============================================
# 最終映像
# ============================================
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /smtp-receiver .

# SMTP 服務埠號
EXPOSE 2525
EXPOSE 1587

CMD ["./smtp-receiver"]

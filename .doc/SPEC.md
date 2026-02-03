---
title: '[Mail Proxy] 企業級分散式郵件發送系統規格書'

---

# [Mail Proxy] 企業級分散式郵件發送系統規格書

## 0. 架構圖
```mermaid
graph TB
    subgraph "Client Layer"
        A1[API Client - Department]
        A2[API Client - Project]
        A3[SMTP Client - SQL Server]
    end

    subgraph "Load Balancer Layer<br/>(Ubuntu VM)"
        LB[HAProxy]
    end

    subgraph "Application Layer<br/>(Docker Containers)"
        MS1[Mail Service 1<br/>Golang Gin API]
    end

    subgraph "Queue Layer<br/>(Docker Container)"
        MQ[(RabbitMQ)]
    end

    subgraph "SMTP Inbound Layer<br/>(Docker Container)"
        SMTP[SMTP Receiver<br/>Port 2525/1587]
    end

    subgraph "Worker Layer<br/>(Docker Containers)"
        W1[Worker Pool<br/>Golang RabbitMQ Consumer]
        ROUTER[MailRouter<br/>網域路由]
    end

    subgraph "Mail Sending Layer"
        OAUTH[Microsoft OAuth 2.0<br/>Authentication]
        MSMTP[Microsoft Graph API<br/>graph.microsoft.com]
        SG[SendGrid API<br/>api.sendgrid.com]
    end

    subgraph "Storage Layer<br/>(Docker Containers)"
        DB[(PostgreSQL<br/>Mail Records)]
        KEYDB[(KeyDB<br/>Status Cache)]
        ATTACH[/Volume Mount<br/>Attachments/]
    end

    A1 & A2 --> LB
    LB --> MS1
    MS1 --> MQ
    MS1 --> DB
    MS1 --> ATTACH
    MS1 --> KEYDB
    
    A3 --> SMTP
    SMTP -->|解析 MIME| MQ
    SMTP --> DB
    SMTP --> KEYDB
    SMTP --> ATTACH
    
    MQ --> W1
    W1 --> ROUTER
    
    ROUTER -->|組織網域| OAUTH
    OAUTH --> MSMTP
    ROUTER -->|非組織網域| SG
    W1 --> DB
    W1 --> KEYDB
    W1 --> ATTACH
    
    style LB fill:#e1f5ff
    style MQ fill:#fff4e1
    style DB fill:#e8f5e9
    style KEYDB fill:#fce4ec
    style ATTACH fill:#e3f2fd
    style ROUTER fill:#fff9c4
    style SG fill:#e8f5e9
    style SMTP fill:#e8f5e9
```

---

## 1. 專案概述

### 1.1 專案名稱
企業級分散式郵件發送系統 (Enterprise Distributed Mail Proxy Service)

### 1.2 專案目標
建立一個高可用、完全自建的企業級郵件發送系統，所有基礎設施均部署於企業內部 Ubuntu VM 環境，支援 **雙郵件路由**：
- **組織網域 (`@ptc-nec.com.tw`)**: 透過 Microsoft Graph API 發送
- **非組織網域**: 透過 SendGrid 第三方服務發送

支援公司內部各部門的郵件發送需求，峰值處理能力達每分鐘 1000 封郵件。

### 1.3 核心需求
- **高峰期處理能力**：每分鐘 1000 封郵件
- **全地端部署**：所有服務運行於企業內部 Ubuntu VM (Docker 容器)
- **水平擴展能力**：支援動態增減 Docker 容器節點
- **雙郵件路由**：根據寄件者網域自動選擇 Graph API 或 SendGrid
- **Microsoft OAuth 2.0**：使用微軟認證機制發送組織郵件
- **風險控制**：避免被標記為垃圾郵件
- **高可用性**：99.9% SLA 保證

### 1.4 技術堆疊
| 層級 | 技術選型 | 部署方式 |
|------|----------|----------|
| 應用層 API | Golang + Gin | Docker Container |
| Worker 層 | Golang + RabbitMQ Consumer | Docker Container |
| 消息隊列 | RabbitMQ 3.12+ | Docker Container |
| 資料庫 | PostgreSQL 15+ | Docker Container |
| 狀態快取 | KeyDB | Docker Container |
| 附件儲存 | Volume Mount (外接硬碟) | Docker Volume |
| 負載均衡 | Nginx/HAProxy | Ubuntu VM |
| 監控 | Prometheus + Grafana | 後續階段 |
| 告警 | AlertManager | 後續階段 |
| 追蹤 | Jaeger | 後續階段 |

> **注意**: 監控 (Prometheus + Grafana)、告警 (AlertManager)、追蹤 (Jaeger) 在本階段暫不執行，後續階段有需要再補做。

---

## 2. 系統架構設計

### 2.1 架構層級

#### 2.1.1 客戶端層 (Client Layer)
- 各部門透過 RESTful API 提交郵件請求
- 支援同步/異步發送模式
- 提供 RESTful API 簡化整合流程
- **JWT Token 永久有效，記錄來源 Client 識別資訊**

#### 2.1.2 SMTP Inbound 層 (SMTP Inbound Layer)
- **Golang + go-smtp** 實作 SMTP 伺服器
- 監聽 Port 2525 (SMTP) 和 1587 (TLS)
- 接收外部 SMTP 郵件並解析 MIME 格式
- 轉發至 RabbitMQ 佇列
- 支援可選認證與寄件網域白名單

#### 2.1.3 負載均衡層 (Load Balancer)
- **自建 Ubuntu VM**: HAProxy 或 Nginx
- 支援健康檢查 (Health Check)
- SSL/TLS 終止 (Let's Encrypt 或自簽憑證)
- 連接數限制
- 配置範例路徑: `/etc/haproxy/` 或 `/etc/nginx/`

#### 2.1.4 應用服務層 (Application Layer)
- **Golang + Gin 開發的微服務**
- **Docker 容器化部署**
- 無狀態設計，支援水平擴展
- 郵件請求驗證與預處理
- 附件處理與儲存 (Volume Mount)

#### 2.1.5 消息隊列層 (Queue Layer)
- **Docker Container**: RabbitMQ
- 單一隊列 (不分優先級)
- 持久化保證 (Durable Queues)
- 死信隊列 (DLX) 處理失敗訊息

#### 2.1.6 工作處理層 (Worker Layer)
- **Golang** 開發
- **Docker 容器化部署**
- 使用 Goroutine 實現並發
- 動態調整 Worker 數量
- 失敗重試機制 (5 次)
- 優雅關機支援 (Graceful Shutdown)
- **透過 Microsoft OAuth 2.0 認證後發送郵件**

#### 2.1.7 Microsoft SMTP 發送層 (SMTP Layer)
- **Microsoft Graph API / SMTP OAuth 2.0**
- 使用 `graph.microsoft.com` 發送郵件
- Application Permission (Client Credentials Flow)
- 發送狀態追蹤

#### 2.1.8 儲存層 (Storage Layer)
- **Docker Container**: PostgreSQL
  - 郵件元數據、發送記錄、JWT Client 記錄
- **Docker Container**: KeyDB
  - 寄信事件 ID 與狀態快取
  - TTL 14 天 (可設定)
- **Docker Volume Mount**: 外接硬碟
  - 附件儲存
  - 單檔限制 25MB (可設定)

---

## 3. 核心功能需求

### 3.1 郵件發送 API

#### 3.1.1 端點設計
```
POST /api/v1/mail/send
POST /api/v1/mail/send/batch
GET  /api/v1/mail/status/:id
GET  /api/v1/mail/history
DELETE /api/v1/mail/cancel/:id
```

#### 3.1.2 請求參數
```json
// POST /api/v1/mail/send
{
  "from": "sender@company.com",
  "to": ["recipient1@example.com", "recipient2@example.com"],
  "cc": ["cc@example.com"],
  "bcc": ["bcc@example.com"],
  "subject": "郵件主旨",
  "body": "郵件內容",
  "html": "<html>...</html>",
  "attachments": [
    {
      "filename": "document.pdf",
      "content": "base64_encoded_content",
      "content_type": "application/pdf"
    }
  ],
  "metadata": {
    "department": "sales",
    "campaign_id": "Q1_2026"
  }
}
```

#### 3.1.3 響應格式
```json
// POST /api/v1/mail/send
{
  "success": true,
  "mail_id": "uuid-v4-string",
  "status": "queued",
  "message": "郵件已加入發送隊列",
  "estimated_send_time": "2026-01-20T10:05:00Z"
}
```

### 3.2 JWT Token 認證機制

#### 3.2.1 Token 設計
- **永久有效**: JWT Token 不設置過期時間 (`exp` claim 不設定)
- **Client 識別**: Token 內含 `client_id`、`client_name`、`department` 等識別資訊
- **簽發記錄**: 所有簽發的 Token 記錄於 PostgreSQL

#### 3.2.2 Token Payload 結構
```json
{
  "iss": "mail-proxy-system",
  "sub": "client_uuid",
  "iat": 1737216000,
  "client_id": "dept_sales_001",
  "client_name": "Sales Department API Client",
  "department": "sales",
  "permissions": ["mail:send", "mail:read", "mail:cancel"]
}
```

#### 3.2.3 Token 管理 API
```
POST   /api/v1/auth/token          # 建立新 Token
GET    /api/v1/auth/token/:id      # 查詢 Token 資訊
DELETE /api/v1/auth/token/:id      # 撤銷 Token
GET    /api/v1/auth/tokens         # 列出所有 Token
```

#### 3.2.4 Client 追蹤
每次 API 呼叫都會記錄:
- `client_id`: Client 唯一識別碼
- `client_name`: Client 名稱
- `request_ip`: 請求來源 IP
- `request_timestamp`: 請求時間
- `mail_id`: 關聯的郵件 ID

### 3.3 狀態查詢 API (KeyDB)

#### 3.3.1 端點
```
GET /api/v1/mail/status/:id
```

#### 3.3.2 響應格式
```json
{
  "mail_id": "uuid-v4-string",
  "status": "sent",
  "retry_count": 0,
  "last_updated": "2026-01-20T10:05:00Z",
  "error_message": null
}
```

#### 3.3.3 狀態值說明
| 狀態 | 說明 |
|------|------|
| queued | 已加入隊列等待處理 |
| processing | 正在處理中 |
| sent | 發送成功 |
| failed | 發送失敗 (已達最大重試次數) |
| cancelled | 已取消 |

#### 3.3.4 KeyDB 設定
- **TTL**: 14 天 (可透過 `.env` 設定 `KEYDB_STATUS_TTL_DAYS`)
- **Key 格式**: `mail:status:{mail_id}`

### 3.4 速率限制策略

#### 3.4.1 多層級限流
- API 層級：每部門每分鐘 200 請求
- 用戶層級：每 Client 每小時 500 封郵件
- 收件人層級：相同收件人每小時 10 封
- SMTP 層級：每分鐘發送限制 (Microsoft 限制)

### 3.5 失敗重試機制

#### 3.5.1 重試策略
- **指數退避演算法**：1s, 2s, 4s, 8s, 16s
- **最大重試次數**：5 次
- **永久失敗處理**：記錄到失敗 Topic，更新 PostgreSQL 狀態

#### 3.5.2 失敗類型處理
| 失敗類型 | 處理方式 |
|----------|----------|
| 暫時性失敗 (網路超時、SMTP 4xx) | 進入重試隊列 |
| 永久性失敗 (無效郵箱、SMTP 5xx) | 標記失敗，不重試 |
| 速率限制 | 延遲重試，調整發送速率 |

#### 3.5.3 失敗記錄流程
```mermaid
flowchart TD
    A[Worker 收到訊息] --> B{發送郵件}
    B -->|成功| C[更新 KeyDB: sent<br/>更新 PostgreSQL: sent]
    B -->|失敗| D{重試次數 < 5?}
    D -->|是| E[發送到 retry topic<br/>指數退避延遲]
    E --> A
    D -->|否| F[發送到 failed topic]
    F --> G[更新 KeyDB: failed<br/>更新 PostgreSQL: failed]
```

---

## 4. Golang 技術實現

### 4.1 核心套件選擇

#### 4.1.1 Web 框架
- **Gin**: 高性能 HTTP 框架

#### 4.1.2 消息隊列
- **github.com/rabbitmq/amqp091-go**: RabbitMQ 官方 Go 客戶端

#### 4.1.3 郵件處理
- **github.com/xhit/go-simple-mail/v2**: SMTP 客戶端 (支援 OAuth2)
- **Azure AD OAuth 2.0**: Client Credentials Flow

#### 4.1.4 資料庫
- **github.com/lib/pq** 或 **github.com/jackc/pgx**: PostgreSQL 驅動
- **GORM**: ORM 選項

#### 4.1.5 快取
- **github.com/go-redis/redis/v8**: KeyDB 客戶端 (Redis 相容)

#### 4.1.6 認證
- **github.com/golang-jwt/jwt/v5**: JWT Token 處理

#### 4.1.7 其他工具
- **github.com/go-playground/validator/v10**: 請求驗證
- **github.com/google/uuid**: UUID 生成
- **github.com/joho/godotenv**: 環境變數載入

### 4.2 專案結構

```
mail-proxy/
├── .docker/
│   ├── api/
│   │   └── Dockerfile
│   ├── worker/
│   │   └── Dockerfile
│   ├── smtp-receiver/
│   │   └── Dockerfile          # SMTP Inbound Server
│   ├── docker-compose.yml
│   └── .env.example
├── cmd/
│   ├── api/
│   │   └── main.go          # Gin RESTful API 入口
│   ├── worker/
│   │   └── main.go          # RabbitMQ Worker 入口
│   └── smtp-receiver/
│       └── main.go          # SMTP Inbound Server 入口
├── internal/
│   ├── api/
│   │   ├── handlers/
│   │   │   ├── mail_handler.go
│   │   │   ├── auth_handler.go
│   │   │   └── status_handler.go
│   │   ├── middlewares/
│   │   │   ├── auth.go
│   │   │   ├── rate_limit.go
│   │   │   └── logger.go
│   │   ├── routes/
│   │   │   └── routes.go
│   │   └── validators/
│   │       └── mail_validator.go
│   ├── smtp/                   # SMTP Inbound Server
│   │   ├── server.go           # SMTP 伺服器核心
│   │   ├── session.go          # Session 處理與 MIME 解析
│   │   └── backend.go          # Backend 介面實作
│   ├── worker/
│   │   ├── consumer.go
│   │   ├── processor.go
│   │   └── retry.go
│   ├── services/
│   │   ├── mail_sender.go       # MailSender interface
│   │   ├── mail_router.go       # 網域路由服務
│   │   ├── smtp_service.go      # GraphMailService (Microsoft Graph API)
│   │   ├── sendgrid_service.go  # SendGridService
│   │   ├── queue_service.go
│   │   ├── keydb_service.go
│   │   └── admin_token_service.go
│   ├── models/
│   │   ├── mail.go
│   │   └── token.go
│   └── config/
│       └── config.go
├── pkg/
│   └── microsoft/
│       └── oauth.go          # Microsoft OAuth 2.0 實作
├── migrations/
│   └── 001_init_schema.sql
├── go.mod
├── go.sum
└── .env.example
```

### 4.3 環境變數設定

#### .env.example
```env
# Server
APP_ENV=development
API_PORT=8080
WORKER_CONCURRENCY=10
WORKER_PREFETCH=10

# PostgreSQL
DATABASE_URL=postgres://user:password@localhost:5432/mail_proxy?sslmode=disable

# RabbitMQ
RABBITMQ_URL=amqp://user:password@localhost:5672/
MAIL_QUEUE_NAME=mail-queue
RETRY_QUEUE_NAME=retry-queue
FAILED_QUEUE_NAME=failed-mails
MAX_RETRY_COUNT=5

# KeyDB
KEYDB_URL=localhost:6379
KEYDB_PASSWORD=
KEYDB_STATUS_TTL_DAYS=14

# Microsoft OAuth 2.0 (組織網域郵件發送)
MICROSOFT_TENANT_ID=your-tenant-id
MICROSOFT_CLIENT_ID=your-client-id
MICROSOFT_CLIENT_SECRET=your-client-secret

# SendGrid (非組織網域郵件發送)
SENDGRID_API_KEY=your-sendgrid-api-key
ORG_EMAIL_DOMAIN=@ptc-nec.com.tw

# Attachment
ATTACHMENT_PATH=/app/attachments
MAX_ATTACHMENT_SIZE_MB=25

# JWT
JWT_SECRET=your-jwt-secret-key

# SMTP Inbound Server
SMTP_INBOUND_PORT=2525
SMTP_INBOUND_TLS_PORT=1587
SMTP_TLS_ENABLED=false
SMTP_AUTH_REQUIRED=false
SMTP_ALLOWED_DOMAINS=
SMTP_MAX_MESSAGE_SIZE_MB=25
```

---

## 5. Docker 配置

### 5.1 目錄結構
```
.docker/
├── api/
│   └── Dockerfile
├── worker/
│   └── Dockerfile
├── docker-compose.yml
└── .env.example
```

### 5.2 API Dockerfile

```dockerfile
# .docker/api/Dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 安裝必要工具
RUN apk add --no-cache git ca-certificates

# 複製 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 複製原始碼
COPY . .

# 編譯
RUN CGO_ENABLED=0 GOOS=linux go build -o /api ./cmd/api

# 最終映像
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /api .

EXPOSE 8080

CMD ["./api"]
```

### 5.3 Worker Dockerfile

```dockerfile
# .docker/worker/Dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /worker ./cmd/worker

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /worker .

CMD ["./worker"]
```

### 5.4 docker-compose.yml

```yaml
# .docker/docker-compose.yml
version: '3.8'

services:
  # PostgreSQL
  postgresql:
    image: postgres:15-alpine
    container_name: mail-proxy-postgresql
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    volumes:
      - postgresql_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 256M
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER}"]
      interval: 10s
      timeout: 5s
      retries: 5

  # KeyDB
  keydb:
    image: eqalpha/keydb:latest
    container_name: mail-proxy-keydb
    restart: unless-stopped
    command: keydb-server --appendonly yes
    volumes:
      - keydb_data:/data
    ports:
      - "6379:6379"
    deploy:
      resources:
        limits:
          cpus: '0.25'
          memory: 256M
        reservations:
          cpus: '0.1'
          memory: 128M
    healthcheck:
      test: ["CMD", "keydb-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  # RabbitMQ
  rabbitmq:
    image: rabbitmq:3.12-management-alpine
    container_name: mail-proxy-rabbitmq
    restart: unless-stopped
    environment:
      RABBITMQ_DEFAULT_USER: ${RABBITMQ_USER}
      RABBITMQ_DEFAULT_PASS: ${RABBITMQ_PASSWORD}
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq
    ports:
      - "5672:5672"
      - "15672:15672"
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 256M
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "status"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 30s

  # API Service
  api:
    build:
      context: ..
      dockerfile: .docker/api/Dockerfile
    container_name: mail-proxy-api
    restart: unless-stopped
    environment:
      - APP_ENV=${APP_ENV}
      - API_PORT=8080
      - DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgresql:5432/${POSTGRES_DB}?sslmode=disable
      - RABBITMQ_URL=amqp://${RABBITMQ_USER}:${RABBITMQ_PASSWORD}@rabbitmq:5672/
      - KEYDB_URL=keydb:6379
      - KEYDB_STATUS_TTL_DAYS=${KEYDB_STATUS_TTL_DAYS}
      - MICROSOFT_TENANT_ID=${MICROSOFT_TENANT_ID}
      - MICROSOFT_CLIENT_ID=${MICROSOFT_CLIENT_ID}
      - MICROSOFT_CLIENT_SECRET=${MICROSOFT_CLIENT_SECRET}
      - ATTACHMENT_PATH=/app/attachments
      - MAX_ATTACHMENT_SIZE_MB=${MAX_ATTACHMENT_SIZE_MB}
      - JWT_SECRET=${JWT_SECRET}
    volumes:
      - ${ATTACHMENT_VOLUME_PATH}:/app/attachments
    ports:
      - "8080:8080"
    depends_on:
      postgresql:
        condition: service_healthy
      keydb:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 256M
        reservations:
          cpus: '0.25'
          memory: 128M
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  # Worker Service
  worker:
    build:
      context: ..
      dockerfile: .docker/worker/Dockerfile
    container_name: mail-proxy-worker
    restart: unless-stopped
    environment:
      - APP_ENV=${APP_ENV}
      - DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgresql:5432/${POSTGRES_DB}?sslmode=disable
      - RABBITMQ_URL=amqp://${RABBITMQ_USER}:${RABBITMQ_PASSWORD}@rabbitmq:5672/
      - KEYDB_URL=keydb:6379
      - KEYDB_STATUS_TTL_DAYS=${KEYDB_STATUS_TTL_DAYS}
      - MICROSOFT_TENANT_ID=${MICROSOFT_TENANT_ID}
      - MICROSOFT_CLIENT_ID=${MICROSOFT_CLIENT_ID}
      - MICROSOFT_CLIENT_SECRET=${MICROSOFT_CLIENT_SECRET}
      - ATTACHMENT_PATH=/app/attachments
      - MAX_ATTACHMENT_SIZE_MB=${MAX_ATTACHMENT_SIZE_MB}
      - WORKER_CONCURRENCY=${WORKER_CONCURRENCY}
      - WORKER_PREFETCH=${WORKER_PREFETCH}
      - MAX_RETRY_COUNT=${MAX_RETRY_COUNT}
    volumes:
      - ${ATTACHMENT_VOLUME_PATH}:/app/attachments
    depends_on:
      postgresql:
        condition: service_healthy
      keydb:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 256M
        reservations:
          cpus: '0.25'
          memory: 128M

volumes:
  postgresql_data:
  keydb_data:
  rabbitmq_data:
```

### 5.5 .env.example (Docker Compose)

```env
# .docker/.env.example

# Environment
APP_ENV=development

# PostgreSQL
POSTGRES_USER=smtp_user
POSTGRES_PASSWORD=your_secure_password
POSTGRES_DB=smtp_service

# RabbitMQ
RABBITMQ_USER=admin
RABBITMQ_PASSWORD=your_secure_password

# KeyDB
KEYDB_STATUS_TTL_DAYS=14

# Microsoft OAuth 2.0
MICROSOFT_TENANT_ID=your-tenant-id
MICROSOFT_CLIENT_ID=your-client-id
MICROSOFT_CLIENT_SECRET=your-client-secret

# Attachment Volume (外接硬碟路徑)
ATTACHMENT_VOLUME_PATH=/mnt/external-disk/smtp-attachments
MAX_ATTACHMENT_SIZE_MB=25

# Worker
WORKER_CONCURRENCY=10
WORKER_PREFETCH=10
MAX_RETRY_COUNT=5

# JWT
JWT_SECRET=your-jwt-secret-key-change-this-in-production
```

---

## 6. API 文件

### 6.1 公開端點 (Public Endpoints)

#### 6.1.1 健康檢查
`GET /health`

用於系統健康狀態監控，**無需認證**。

**回應範例 (Healthy):**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "services": {
    "postgresql": "ok",
    "keydb": "ok",
    "rabbitmq": "ok"
  }
}
```

**回應範例 (Degraded - HTTP 503):**
```json
{
  "status": "degraded",
  "version": "1.0.0",
  "services": {
    "postgresql": "ok",
    "keydb": "error",
    "rabbitmq": "ok"
  }
}
```

---

### 6.2. 認證與授權 (Authentication & Authorization)

本系統使用 **JWT (JSON Web Token)** 進行 API 存取控制，每一部門或部門內每一專案為 JWT 發放單位，無須先行登入，發放用即可永久使用，若須依集團資安規範而撤銷更替，MIS 會先行通知協作。

#### 6.2.1 認證方式
所有需要認證的 API 必須在 HTTP Header 中攜帶 Bearer Token：
```http
Authorization: Bearer <Your_JWT_Token>
```

#### 6.2.2 權限限制 (RBAC)
| 權限層級 | 可存取端點 | 說明 |
| :--- | :--- | :--- |
| **一般 Client** | `/api/v1/mail/*` | 郵件發送、狀態查詢、歷史紀錄 |
| **Admin** | `/api/v1/mail/*` + `/api/v1/auth/*` | 額外具備 Token 管理權限 |

#### 6.2.3 錯誤回應
| HTTP Code | 錯誤代碼 | 說明 |
| :---: | :--- | :--- |
| 401 | `missing_token` | 未提供 Authorization Header |
| 401 | `invalid_token_format` | Token 格式錯誤，需使用 Bearer |
| 401 | `invalid_token` | Token 無效 |
| 401 | `token_revoked` | Token 已被撤銷 |
| 403 | `permission_denied` | 權限不足 |

---

### 6.3. 郵件相關 API (Mail API)

> **認證要求**: 需攜帶有效的 JWT Token

#### 6.3.1 發送單封郵件
`POST /api/v1/mail/send`

發送單一電子郵件。

**請求參數 (Request Body):**
| 欄位 | 類型 | 必填 | 說明 |
| :--- | :--- | :---: | :--- |
| `from` | string | ✓ | 發件人 Email |
| `to` | string[] | ✓ | 收件人 Email 列表 (至少一筆) |
| `cc` | string[] | | 副本收件人 |
| `bcc` | string[] | | 密件副本收件人 |
| `subject` | string | ✓ | 郵件主旨 |
| `body` | string | | 純文字客戶端/無障礙閱讀器，純文字內容 |
| `html` | string | | 現代郵件客戶端，HTML 格式內容 |
| `attachments` | object[] | | 附件列表 |
| └ `filename` | string | ✓ | 檔案名稱 |
| └ `content` | string | ✓ | 檔案內容 (Base64 編碼) |
| └ `content_type`| string | | MIME 類型 (如 `application/pdf`) |
| `metadata` | object | | 自定義擴充資訊 |

> body、html 同時提供兩者是最佳做法，確保所有收件人都能正確閱讀郵件

**請求範例:**
```json
{
  "from": "sender@example.com",
  "to": ["receiver@example.com"],
  "cc": ["cc@example.com"],
  "subject": "歡迎加入！",
  "body": "歡迎加入我們的平台！請點擊以下連結啟用帳號：https://example.com/activate",
  "html": "<h1>歡迎加入！</h1><p>請點擊以下按鈕啟用帳號：</p><a href='https://example.com/activate' style='background:#007bff;color:white;padding:10px 20px;'>啟用帳號</a>",
  "attachments": [
    {
      "filename": "hello.txt",
      "content": "SGVsbG8gV29ybGQ=",
      "content_type": "text/plain"
    }
  ],
  "metadata": {
    "campaign_id": "2026Q1"
  }
}
```

**回應範例 (Success - 200):**
```json
{
  "success": true,
  "mail_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "queued",
  "message": "郵件已加入發送隊列"
}
```

**錯誤回應範例 (400):**
```json
{
  "success": false,
  "error": "attachment_too_large",
  "message": "report.pdf exceeds maximum size of 25MB"
}
```

---

#### 6.3.2 批次發送郵件
`POST /api/v1/mail/send/batch`

一次發送多封電子郵件。

**請求範例:**
```json
{
  "mails": [
    {
      "from": "sender@example.com",
      "to": ["receiver@example.com"],
      "cc": ["cc@example.com"],
      "subject": "通知 #1",
      "body": "這是第一封郵件",
      "html": "<h1>這是第一封郵件</h1>",
      "attachments": [
        {
          "filename": "hello.txt",
          "content": "SGVsbG8gV29ybGQ=",
          "content_type": "text/plain"
        }
      ],
      "metadata": {
        "campaign_id": "2026Q1"
      }
    },
    {
      "from": "sender@example.com",
      "to": ["receiver@example.com"],
      "cc": ["cc@example.com"],
      "subject": "通知 #2",
      "body": "這是第二封郵件"
      "html": "<h1>這是第二封郵件</h1>",
      "attachments": [
        {
          "filename": "hello.txt",
          "content": "SGVsbG8gV29ybGQ=",
          "content_type": "text/plain"
        }
      ],
      "metadata": {
        "campaign_id": "2026Q1"
      }
    }
  ]
}
```

**回應範例 (Success - 200):**
```json
{
  "success": true,
  "batch_id": "b8f3e1a0-1234-5678-abcd-ef1234567890",
  "results": [
    { "mail_id": "550e8400-...", "status": "queued" },
    { "mail_id": "660f9500-...", "status": "queued" }
  ]
}
```

---

#### 6.3.3 查詢郵件狀態
`GET /api/v1/mail/status/:id`

**路徑參數:**
| 參數 | 說明 |
| :--- | :--- |
| `id` | 郵件 UUID |

**可能的狀態值:**
| 狀態 | 說明 |
| :--- | :--- |
| `queued` | 已進入佇列，等待處理 |
| `processing` | Worker 正在處理中 |
| `sent` | 發送成功 |
| `failed` | 發送失敗 (已達重試上限) |
| `cancelled` | 已取消 |

**回應範例:**
```json
{
  "mail_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "sent",
  "retry_count": 0,
  "created_at": "2026-01-19T01:00:00Z",
  "sent_at": "2026-01-19T01:00:05Z",
  "error_message": ""
}
```

---

#### 6.3.4 查詢郵件歷史
`GET /api/v1/mail/history`

查詢當前 Client 的郵件發送歷史紀錄。

**查詢參數:**
| 參數 | 類型 | 預設值 | 說明 |
| :--- | :--- | :---: | :--- |
| `page` | integer | 1 | 頁碼 |
| `limit` | integer | 20 | 每頁筆數 (最大 100) |
| `status` | string | | 過濾狀態 (可選) |

**請求範例:**
```
GET /api/v1/mail/history?page=1&limit=10&status=sent
```

**回應範例:**
```json
{
  "total": 150,
  "page": 1,
  "limit": 10,
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "from": "sender@example.com",
      "to": ["receiver@example.com"],
      "subject": "測試郵件",
      "status": "sent",
      "created_at": "2026-01-19T01:00:00Z",
      "sent_at": "2026-01-19T01:00:05Z"
    }
  ]
}
```

---

#### 6.3.5 取消發送
`DELETE /api/v1/mail/cancel/:id`

取消一封 **尚在佇列中** 的郵件。

**限制條件:**
- 只有狀態為 `queued` 的郵件可以取消
- 正在處理中 (`processing`) 或已發送 (`sent`) 的郵件無法取消

**回應範例 (Success - 200):**
```json
{
  "success": true,
  "mail_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "cancelled",
  "message": "郵件已取消"
}
```

**錯誤回應範例 (400):**
```json
{
  "success": false,
  "error": "cannot_cancel",
  "message": "Only queued mails can be cancelled"
}
```

---

### 6.4. Token 管理 API (Admin Only)

> **認證要求**: 需攜帶具備 `admin` 權限的 JWT Token

#### 6.4.1 建立 Client Token
`POST /api/v1/auth/token`

為新的 Client 系統建立 API Token。

**請求參數:**
| 欄位 | 類型 | 必填 | 說明 |
| :--- | :--- | :---: | :--- |
| `client_name` | string | ✓ | Client 名稱 |
| `department` | string | | 部門名稱 |
| `permissions` | string[] | ✓ | 權限列表 (如 `["mail.send", "mail.read", "mail.cancel", "admin"]`) |

**請求範例:**
```json
{
  "client_name": "行銷部門系統",
  "department": "Marketing",
  "permissions": ["mail.send", "mail.read", "mail.cancel"]
}
```

**回應範例 (Success - 201):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "client_id": "client_abc12345",
  "created_at": "2026-01-19T09:00:00Z"
}
```

> ⚠️ **重要**: Token 只會在建立時顯示一次，請妥善保存。

---

#### 6.4.2 查詢 Token 資訊
`GET /api/v1/auth/token/:id`

**路徑參數:**
| 參數 | 說明 |
| :--- | :--- |
| `id` | Token UUID 或 `client_id` |

**回應範例:**
```json
{
  "id": "a1b2c3d4-1234-5678-abcd-ef1234567890",
  "client_id": "client_abc12345",
  "client_name": "行銷部門系統",
  "department": "Marketing",
  "permissions": ["mail.send"],
  "is_active": true,
  "created_at": "2026-01-19T09:00:00Z",
  "revoked_at": null
}
```

---

#### 6.4.3 撤銷 Token
`DELETE /api/v1/auth/token/:id`

撤銷指定的 Client Token，使其立即失效。

**回應範例 (Success - 200):**
```json
{
  "success": true,
  "message": "Token 已撤銷"
}
```

---

#### 6.4.4 列出所有 Token
`GET /api/v1/auth/tokens`

列出系統中所有已建立的 Client Token。

**回應範例:**
```json
{
  "total": 5,
  "data": [
    {
      "id": "a1b2c3d4-1234-5678-abcd-ef1234567890",
      "client_id": "client_abc12345",
      "client_name": "行銷部門系統",
      "department": "Marketing",
      "permissions": ["mail.send"],
      "is_active": true,
      "created_at": "2026-01-19T09:00:00Z"
    },
    {
      "id": "b2c3d4e5-2345-6789-bcde-fg2345678901",
      "client_id": "client_xyz98765",
      "client_name": "HR 系統",
      "department": "Human Resources",
      "permissions": ["mail.send"],
      "is_active": false,
      "created_at": "2026-01-15T10:00:00Z",
      "revoked_at": "2026-01-18T14:30:00Z"
    }
  ]
}
```
---

## 7. Worker 運作說明

### 7.1 Worker 啟動流程

```mermaid
sequenceDiagram
    participant W as Worker
    participant RMQ as RabbitMQ
    participant OAuth as Microsoft OAuth
    participant SMTP as Microsoft SMTP
    participant KeyDB as KeyDB
    participant PG as PostgreSQL

    W->>RMQ: 連接並訂閱 mail-queue
    loop 持續監聽
        RMQ->>W: 推送郵件任務
        W->>KeyDB: 更新狀態為 processing
        W->>OAuth: 請求 Access Token
        OAuth-->>W: 返回 Token
        W->>SMTP: 發送郵件 (OAuth2 認證)
        alt 發送成功
            SMTP-->>W: 成功回應
            W->>KeyDB: 更新狀態為 sent
            W->>PG: 更新郵件記錄
            W->>RMQ: ACK 消息
        else 發送失敗
            SMTP-->>W: 錯誤回應
            W->>W: 檢查重試次數
            alt 重試次數 < 5
                W->>RMQ: 發送到 retry-queue (帶延遲)
            else 重試次數 >= 5
                W->>RMQ: 發送到 failed-mails
                W->>KeyDB: 更新狀態為 failed
                W->>PG: 更新郵件記錄
            end
            W->>RMQ: ACK 消息
        end
    end
```

### 7.2 重試機制

| 重試次數 | 延遲時間 | 累計等待 |
|----------|----------|----------|
| 1 | 1 秒 | 1 秒 |
| 2 | 2 秒 | 3 秒 |
| 3 | 4 秒 | 7 秒 |
| 4 | 8 秒 | 15 秒 |
| 5 | 16 秒 | 31 秒 |

### 7.3 RabbitMQ 隊列配置

| 隊列名稱 | 說明 | 特性 |
|----------|------|------|
| mail-queue | 主郵件隊列 | durable, 死信交換器指向 dlx |
| retry-queue | 重試隊列 | durable, 支援延遲消息 |
| failed-mails | 失敗郵件隊列 | durable, 手動處理 |

### 7.4 系統日誌 (Docker Logs)

所有日誌輸出到 stdout/stderr，可透過 Docker 日誌查詢：

```bash
# 查看 API 日誌
docker logs mail-proxy-api

# 查看 Worker 日誌
docker logs mail-proxy-worker

# 實時追蹤日誌
docker logs -f mail-proxy-worker

# 查看最近 100 行
docker logs --tail 100 mail-proxy-worker
```

日誌格式 (JSON):
```json
{
  "time": "2026-01-20T10:00:00Z",
  "level": "info",
  "msg": "Mail sent successfully",
  "mail_id": "uuid-string",
  "duration_ms": 450
}
```

---

## 8. 資料庫 Schema

### 8.1 PostgreSQL Tables

```sql
-- 建立資料庫
CREATE DATABASE smtp_service;

-- 郵件表
CREATE TABLE mails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_address VARCHAR(255) NOT NULL,
    to_addresses TEXT[] NOT NULL,
    cc_addresses TEXT[],
    bcc_addresses TEXT[],
    subject TEXT NOT NULL,
    body TEXT,
    html TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'queued',
    retry_count INT DEFAULT 0,
    error_message TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    client_id VARCHAR(100) NOT NULL,
    client_name VARCHAR(255),
    metadata JSONB
);

-- Client Token 表
CREATE TABLE client_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id VARCHAR(100) UNIQUE NOT NULL,
    client_name VARCHAR(255) NOT NULL,
    department VARCHAR(100),
    permissions TEXT[] NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT TRUE
);

-- API 請求日誌表
CREATE TABLE api_logs (
    id BIGSERIAL PRIMARY KEY,
    client_id VARCHAR(100) NOT NULL,
    client_name VARCHAR(255),
    request_ip INET,
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(10) NOT NULL,
    mail_id UUID REFERENCES mails(id),
    status_code INT,
    response_time_ms INT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 附件表
CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mail_id UUID REFERENCES mails(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(100),
    size_bytes BIGINT,
    storage_path VARCHAR(500) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 索引
CREATE INDEX idx_mails_status ON mails(status);
CREATE INDEX idx_mails_client_id ON mails(client_id);
CREATE INDEX idx_mails_created_at ON mails(created_at);
CREATE INDEX idx_api_logs_client_id ON api_logs(client_id);
CREATE INDEX idx_api_logs_created_at ON api_logs(created_at);
```

---

## 9. 附件儲存設計

### 9.1 儲存架構

- **儲存位置**: Docker Volume 掛載到 VM 外接硬碟
- **路徑設定**: 透過 `.env` 的 `ATTACHMENT_VOLUME_PATH` 設定
- **單檔限制**: 25MB (可透過 `.env` 的 `MAX_ATTACHMENT_SIZE_MB` 設定)

### 9.2 目錄結構
```
${ATTACHMENT_VOLUME_PATH}/
├── 2026/
│   ├── 01/
│   │   ├── 20/
│   │   │   ├── {mail_id}/
│   │   │   │   ├── document.pdf
│   │   │   │   └── image.png
```

### 9.3 清理策略
- 附件保留時間: 30 天
- 建議設定 cron job 定期清理過期附件

---

## 10. 安全性需求

### 10.1 身份驗證
- **JWT Token (永久有效)**
  - 使用 RS256 非對稱加密
  - Token 內含完整 Client 識別資訊
  - 支援 Token 撤銷機制

### 10.2 授權管理
- RBAC (Role-Based Access Control)
- 部門級別隔離
- API 配額管理
- 權限範圍: `mail:send`, `mail:read`, `mail:cancel`, `admin`

### 10.3 資料加密
- 傳輸加密: TLS 1.3
- 資料庫加密: 敏感欄位 AES-256

### 10.4 敏感資訊保護
- 密碼使用環境變數
- Microsoft OAuth 憑證妥善保管
- 郵件內容不記錄到日誌
- 個資遮罩處理

---

## 11. 部署與 CI/CD

### 11.1 本地開發

```bash
# 複製環境變數
cp .docker/.env.example .docker/.env

# 編輯環境變數
vim .docker/.env

# 啟動所有服務
cd .docker
docker-compose up -d

# 查看日誌
docker-compose logs -f

# 停止服務
docker-compose down
```

### 11.2 容器化
- Docker 映像建構
- Dockerfile 多階段建構優化
- 映像安全掃描 (Trivy)

---

## 12. 附錄

### 12.1 軟體版本建議
| 軟體 | 版本 | 說明 |
|------|------|------|
| Ubuntu | 22.04 LTS | 長期支援版本 |
| Golang | 1.22+ | 最新穩定版 |
| PostgreSQL | 15+ | 最新穩定版 |
| RabbitMQ | 3.12+ | 最新穩定版 |
| KeyDB | latest | Redis 相容 |
| Docker | 24+ | 最新穩定版 |
| Docker Compose | 2.x | 最新穩定版 |

### 13.2 開發工具
- Golang: https://go.dev/
- Gin: https://gin-gonic.com/
- GORM: https://gorm.io/
- amqp091-go: https://github.com/rabbitmq/amqp091-go

### 13.3 Microsoft Graph 參考
- Microsoft Graph API: https://docs.microsoft.com/graph/
- SMTP OAuth 2.0: https://docs.microsoft.com/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth

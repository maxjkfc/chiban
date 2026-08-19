# 吃伴 (Chiban)

和朋友一起記錄飲食。拍照發布，紀錄即時出現在私人群組裡，朋友可以回覆、Reaction、丟 GIF 與貼圖。

- 產品與技術範圍：[`docs/MVP_SPEC.md`](docs/MVP_SPEC.md)
- 開發規格與 slice 計劃：[`docs/V0.1_SPEC.md`](docs/V0.1_SPEC.md)
- 工程約束：[`AGENTS.md`](AGENTS.md)

## 開發環境

需求：Docker、Go 1.26+、Node 22+。

```bash
cp .env.example .env
make dev
```

`make dev` 會建置並啟動 web、api、postgres、fake-gcs。啟動後：

| 服務 | 位置 |
|---|---|
| Web | http://localhost:3000 |
| API | http://localhost:18080 |
| PostgreSQL | localhost:15432 |
| fake-gcs | http://localhost:14443 |

首頁會顯示 API 連線狀態，可用來確認整條 browser → API → PostgreSQL 的路徑是通的。

host port 刻意避開 5432 / 4443 / 8080，以免和機器上其他專案的 compose stack 衝突。要改就改 `.env`。

### 從原始碼跑 api / web

```bash
make run-api    # 起 postgres/fake-gcs，再從原始碼跑 Go API
make run-web
```

`.env` 只放 port、帳密與 token 這些原始值；`CHIBAN_*` 由 Makefile 從它們推導，所以每個 port 只定義在一個地方。改 `.env` 的 `POSTGRES_PORT` 或 `API_PORT`，Compose 與 Makefile 會一起跟著變。

Go 不會自動讀 `.env`，因此請走 make target；要手動跑就照 Makefile 的組法自行 export。`make run-api` 監聽的 port 與 Compose 發佈的相同，所以 web dev server 兩種跑法都接得到——但要先把 api 容器停掉。

### 測試

```bash
make test
```

整合測試會對真實的 PostgreSQL 啟動完整 API，並以 in-memory object storage 取代 fake-gcs；另有一個 smoke test 直接打 fake-gcs，確認 storage 介面在真實後端上成立。

測試會 TRUNCATE 所有資料表並把 migration 退到零，因此跑在獨立的 `chiban_test` 資料庫，不會動到 app 用的 `chiban`。`make test` 會在需要時自動建立這個資料庫。

測試策略與 seam 的理由見 [`docs/V0.1_SPEC.md`](docs/V0.1_SPEC.md) 的 Testing Decisions。

### Migration

```bash
make migrate-up
make migrate-down
```

Migration 檔在 `apps/api/migrations/`，以 goose 格式撰寫並編譯進 binary，因此 API、migrate 指令與測試套件套用的是同一份 schema。API 啟動時會自動套用未執行的 migration。

新增 migration：在該目錄建立 `NNNNN_name.sql`，包含 `-- +goose Up` 與 `-- +goose Down` 兩段。已經套用出去的 migration 不要改，另外新增一支。

## 部署

目標環境是 Mac mini + Docker Compose + Cloudflare Tunnel：

```bash
docker compose -f docker-compose.yml -f docker-compose.deploy.yml --profile tunnel up -d --build
```

網站與 API 共用一個主機名，由 `deploy/Caddyfile` 依路徑分流（`/api/*` 給後端，其餘給前端）。因此沒有 CORS，前端 image 也不含任何網域。PostgreSQL 與 fake-gcs 不對外開放；只有 Cloudflare Tunnel 進得來。secrets 放 `.env`，不進 git。

完整步驟、部署後檢查與備份排程見 [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)。

## Repository

```text
apps/api    Go modular monolith（HTTP + 之後的 WebSocket）
apps/web    Next.js（mobile-first）
docs        規格
```

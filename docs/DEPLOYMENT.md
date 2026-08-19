# 部署（Mac mini + Cloudflare Tunnel）

一台 Mac mini 跑整個 stack，Cloudflare Tunnel 把一個網域接進來。沒有對外開放的連接埠，也不需要固定 IP。

## 拓撲

```text
瀏覽器 → Cloudflare（終止 TLS）→ Tunnel → caddy:80
                                            ├─ /api/*  → api:8080
                                            └─ 其餘     → web:3000
```

**整個產品只有一個主機名。** 路徑分流在 `deploy/Caddyfile` 裡，跟著 repo 走。這個決定帶來三件事：

- 網站與 API 同源，所以沒有 CORS，session cookie 也不必處理 SameSite 的跨站規則。
- `NEXT_PUBLIC_API_BASE_URL` 是空的，前端用相對路徑，所以**建出來的 image 不含任何網域**——換網域不必重建。
- Cloudflare 儀表板只要設一個 public hostname，不必維護多條 ingress 規則。

`postgres` 與 `fake-gcs` 沒有自己的認證，只在 compose 網路裡；`caddy` 也只發布到 `127.0.0.1`，公開流量一律走 tunnel。

## 一次性設定

### 1. 建立 tunnel 並取得 token

在 Cloudflare Zero Trust → Networks → Tunnels 建一個 tunnel，選 **Cloudflared**，複製它給的 token（`eyJ...` 那一長串）。

### 2. 設定 public hostname

在同一個 tunnel 的 **Public Hostname** 分頁新增一筆：

| 欄位 | 值 |
|---|---|
| Subdomain / Domain | 你的網域，例如 `chiban.example.com` |
| Type | `HTTP` |
| URL | `caddy:80` |

只要這一筆。路徑分流由 Caddy 處理，不要在這裡加第二條規則。

Type 是 `HTTP` 而不是 `HTTPS`：TLS 已經在 Cloudflare 終止，tunnel 到 caddy 這段走的是 compose 內部網路。

### 3. 填 `.env`

```bash
cp .env.example .env
```

需要改的只有一項：

```env
CLOUDFLARE_TUNNEL_TOKEN=eyJ...
```

`POSTGRES_PASSWORD` 也請改掉，別用預設值。其餘（網域、cookie、CORS）由 `docker-compose.deploy.yml` 處理，不需要在 `.env` 裡出現。

## 啟動

```bash
docker compose -f docker-compose.yml -f docker-compose.deploy.yml --profile tunnel up -d --build
```

`--build` 是必要的：`docker-compose.deploy.yml` 用建置參數把前端的 API base 設成空字串，沿用舊 image 會帶著別人的 `localhost`。

## 部署後檢查

```bash
scripts/check-exposure.sh
```

確認沒有東西從這台機器外面連得到。應該全部是 `ok`。

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8090/login
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8090/api/v1/auth/me
```

第一個要 `200`（網站），第二個要 `401`（API 有回應，只是沒登入）。兩個都通過代表路徑分流正確。

接著用手機開你的網域，跑一次：註冊 → 建群 → 拍照發布 → 分享 → 另一支手機用邀請連結加入 → 互動。特別確認**聊天室的即時更新**有效——那代表 WebSocket 穿過 tunnel 成功了，是這個拓撲裡最容易出問題的一段。

## 備份

```bash
scripts/backup.sh /Volumes/你的外接碟/chiban
```

排程（每天 04:00）：

```cron
0 4 * * * cd /Users/你/code/chiban && scripts/backup.sh /Volumes/你的外接碟/chiban >> /tmp/chiban-backup.log 2>&1
```

定期真的還原一次到別的資料庫，確認備份是活的：

```bash
scripts/restore.sh /Volumes/你的外接碟/chiban/20260819T040000Z --into chiban_restore_check
```

沒有還原過的備份不算備份。

## 已知界線

- **不是高可用**。單機、單一 Postgres、fake-gcs 當物件儲存。V0.1 要驗的是產品，不是基礎設施。
- **fake-gcs 不是 GCS**。沒有認證、沒有備援，資料在一個 docker volume 裡，備份靠上面那支腳本。
- **備份不是同一時間點的快照**。資料庫先 dump、物件後打包，中間 API 仍可寫入。細節與為什麼接受，寫在 `scripts/backup.sh` 檔頭。
- **升級會中斷服務**。`up -d --build` 期間網站短暫不可用，沒有滾動更新。

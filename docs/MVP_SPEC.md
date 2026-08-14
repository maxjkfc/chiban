# 吃伴（Chiban）V0.1 MVP 需求與技術規格

## 1. 文件目的

本文件定義「吃伴」V0.1 MVP 的產品需求、系統邊界、資料模型、API、權限規則與開發順序，作為 Codex / Claude Code 與人工開發的主要依據。

V0.1 不以熱量計算或完整飲食管理為目標，而是驗證：

> 一群朋友共同記錄飲食，透過群組分享、回覆、Reaction、GIF 與自訂貼圖互動，是否能提升持續記錄與共同飲控的動機。

---

# 2. V0.1 產品定位

吃伴是一個小型、封閉群組的飲食紀錄與社交互動工具。

核心 Loop：

```text
吃東西
↓
快速拍照記錄
↓
自動分享至指定群組
↓
朋友看到
↓
Reaction / Reply / GIF / 貼圖 / 聊天
↓
形成陪伴與社交監督
↓
下一餐再次記錄
```

核心原則：

> Record once, share automatically.

`MealRecord` 是飲食的 Primary Data。聊天室只引用 Meal，不複製 Meal 的內容或圖片路徑。

---

# 3. V0.1 要驗證的問題

第一版主要回答：

1. 使用者是否願意每天快速拍照記錄飲食？
2. 分享到私人群組後，朋友是否會實際互動？
3. Reply / Reaction / GIF / 自訂貼圖是否會增加回訪與記錄頻率？
4. 「一起記錄」是否比個人飲食日誌更有持續性？

V0.1 不驗證：

- 熱量計算是否準確
- TDEE 是否準確
- AI 食物辨識是否準確
- 營養建議是否有效

---

# 4. V0.1 核心使用流程

## 4.1 透過邀請加入

推薦的首次使用流程：

```text
朋友收到群組邀請連結
↓
開啟吃伴
↓
註冊 / 登入
↓
設定暱稱
↓
頭像（可略過）
↓
自動完成加入群組
↓
開始記錄
```

亦可：

```text
註冊
↓
建立 Profile
↓
建立群組
↓
分享邀請連結 / 邀請碼
```

第一版 onboarding 不要求身高、體重、性別、TDEE 或飲控目標。

## 4.2 記錄飲食

```text
點擊「記錄」
↓
拍照 / 選擇照片
↓
確認時間（預設現在）
↓
餐別（optional）
↓
文字備註（optional）
↓
選擇分享群組
↓
發布
```

目標：最短流程應能做到：

```text
拍照 → 發布
```

## 4.3 分享與聊天室

```text
建立 MealRecord
↓
建立 MealGroupShare
↓
建立 meal 類型 ChatMessage
↓
WebSocket broadcast
↓
群組成員即時看到
↓
Reaction / Reply / GIF / Sticker / 一般聊天
```

---

# 5. Authentication

V0.1：

- Email + Password 註冊
- 登入
- 登出
- Session 驗證
- Authentication middleware

Password 不得明文儲存，使用 Argon2id 或 bcrypt。

Session Cookie 應使用適當的：

```text
HttpOnly
Secure（公開 HTTPS 環境）
SameSite
```

暫不實作：

- Google OAuth
- Apple Login
- LINE Login
- MFA

---

# 6. User / Profile

V0.1 Profile 保持極簡：

```text
profiles
--------
user_id
display_name
avatar_media_id nullable
timezone
created_at
updated_at
```

`timezone` 必須保留，例如：

```text
Asia/Taipei
Asia/Tokyo
```

所有 DB timestamp 使用 timezone-aware timestamp（PostgreSQL `timestamptz`），資料庫以 UTC 保存，使用者看到的「今天」與 meal 日期依使用者 timezone 計算。

## 6.1 Profile 隱私

群組可見：

- display_name
- avatar

V0.1 不收集也不公開：

- 身高
- 體重
- 性別
- TDEE
- 每日熱量目標

---

# 7. Group

系統是私人群組，不是公開社群。

```text
groups
------
id
name
owner_id
created_at
updated_at
```

```text
group_members
-------------
group_id
user_id
role
joined_at
```

Role：

```text
owner
member
```

一位使用者可以加入多個 Group。

## 7.1 Group Invite

```text
group_invites
-------------
id
group_id
code
created_by
expires_at
revoked_at
created_at
```

V0.1 規則：

- 邀請預設 7 天失效
- owner 可 revoke
- 同一邀請可多人使用
- 已登入使用者可直接加入
- 未登入者完成註冊後應回到原本 invite flow 並加入群組

## 7.2 Ownership

Owner 不可在未轉移 ownership 的情況下直接離開群組。

V0.1 至少需要保證群組不會進入沒有 owner 的狀態。

---

# 8. Meal Record

`MealRecord` 是產品的核心 Primary Data。

```text
meal_records
------------
id
user_id
meal_type nullable
eaten_at
description nullable
created_at
updated_at
deleted_at nullable
```

Meal Type：

```text
breakfast
lunch
dinner
snack
other
```

V0.1：

必填：

- 至少一張 Meal Photo
- eaten_at（預設現在）

選填：

- meal_type
- description

不要求：

- calories
- protein
- carbs
- fat
- serving size
- AI analysis

---

# 9. Meal Photo

每筆 Meal 支援 1～4 張照片。

```text
meal_photos
-----------
id
meal_record_id
bucket
object_name
content_type
size_bytes
sort_order
created_at
```

Database 不存 binary。

## 9.1 圖片處理

V0.1 至少要做到：

- 驗證實際可解碼的圖片格式，不只相信副檔名/MIME header
- 限制檔案大小
- 限制最大尺寸
- 一般圖片可 resize / 壓縮
- 移除 EXIF / GPS metadata
- 不產生時間浮水印

**飲食圖片時間浮水印明確不屬於 V0.1。**

---

# 10. Object Storage

V0.1 使用：

> fake-gcs-server

運行於 Mac mini Docker 環境。

建議 buckets / namespaces：

```text
meal-images
avatars
chat-media
user-stickers
```

Meal object path 可採：

```text
users/{user_id}/meals/{year}/{month}/{uuid}.{ext}
```

前端不得依賴：

- bucket
- object_name
- fake-GCS URL

Frontend 只認 application-level ID，例如：

```text
image_id
media_id
sticker_id
```

Backend 使用 storage abstraction，例如：

```go
type ObjectStorage interface {
    Upload(...)
    Delete(...)
    Open(...)
}
```

---

# 11. 圖片存取權限

V0.1 圖片讀取：

```text
Browser
↓
GET application media endpoint
↓
Go API
↓
Authentication
↓
Authorization
↓
fake GCS
↓
Go stream
↓
Browser
```

fake GCS 不直接公開給 browser。

禁止：

```text
GET /images?path=users/xxx/secret.webp
```

允許：

```text
GET /api/v1/meal-images/{image_id}
```

Server 根據 `image_id` 自行解析 storage object 並檢查權限。

Signed URL 不屬於 V0.1。

---

# 12. Meal 分享關係

分享權限不可只靠 ChatMessage 反推。

新增正式 relationship：

```text
meal_group_shares
-----------------
meal_record_id
group_id
shared_at
revoked_at nullable
```

Unique：

```text
(meal_record_id, group_id)
```

用途：

- 表示 Meal 是否分享給某 Group
- 作為 meal / meal image authorization 的重要依據
- ChatMessage 只是該分享內容在聊天室中的呈現

建立分享後，建立：

```text
ChatMessage
message_type = meal
meal_record_id = {meal_id}
```

ChatMessage 不複製：

- Meal description
- photo URL / object path
- 其他 Meal fields

---

# 13. Meal / Photo 建立一致性

Object Storage 與 PostgreSQL 無法共用同一個 transaction，因此必須明確處理 partial failure。

建議流程：

```text
建立 Meal
↓
上傳圖片至 fake GCS
↓
建立 MealPhoto metadata
↓
全部圖片成功
↓
建立 MealGroupShare + Meal ChatMessage
↓
commit / publish
```

若 storage 已成功但 DB metadata 寫入失敗：

- best-effort 刪除已上傳 object
- 不可留下對使用者可見的不完整 Meal

應預留 orphan object cleanup 的能力；V0.1 可先以簡單 maintenance script / job 實作，不需 message queue。

---

# 14. Chat

每個 Group 有一個聊天室語意。

V0.1 Message Type：

```text
text
meal
image
gif
sticker
system
```

```text
chat_messages
-------------
id
group_id
user_id
message_type
content nullable
meal_record_id nullable
chat_media_id nullable
sticker_id nullable
reply_to_message_id nullable
client_message_id
created_at
updated_at
deleted_at nullable
```

`client_message_id` 由 client 產生 UUID，用於重送 idempotency。

建議 unique：

```text
(user_id, client_message_id)
```

---

# 15. Reply

不建立 Comment subsystem。

對 Meal 的留言本質為 Chat Reply：

```text
reply_to_message_id
```

任何可回覆的訊息都使用同一機制。

---

# 16. Reaction

```text
message_reactions
-----------------
message_id
user_id
reaction_type
created_at
```

Unique：

```text
(message_id, user_id, reaction_type)
```

V0.1 至少支援：

```text
❤️ 😂 🔥 👏 👀
```

---

# 17. Chat Image / GIF

聊天室可直接傳送：

- 靜態圖片
- GIF

```text
chat_media
----------
id
user_id
media_type
bucket
object_name
content_type
size_bytes
created_at
deleted_at nullable
```

`media_type`：

```text
image
gif
```

GIF 必須保留動畫，不可在一般圖片處理流程中被轉成單張靜態 WebP。

V0.1 不串接 GIPHY / Tenor 搜尋 API。

---

# 18. 自訂 Sticker

使用者可以上傳並保存自己的貼圖。

Sticker 可為：

```text
image
gif
```

```text
user_stickers
-------------
id
user_id
name nullable
media_type
bucket
object_name
content_type
size_bytes
created_at
deleted_at nullable
```

V0.1 支援：

- 新增自己的 Sticker
- 查看「我的貼圖」
- 在聊天室傳送 Sticker
- GIF 可作為 Sticker

聊天室的 sticker message 只 reference `sticker_id`，不複製 storage URL。

可後續再考慮：

- 從別人的 GIF / Sticker 收藏到自己的貼圖
- Sticker pack
- GIF provider 搜尋

上述不列為第一輪必做。

---

# 19. Realtime Chat

V0.1 使用 Go WebSocket。

```text
Browser
↓
WebSocket
↓
Go API
↓
Group Chat Hub
↓
Group members
```

Send flow：

```text
Client
↓
client_message_id
↓
Go 驗證 group membership
↓
DB INSERT（或命中既有 idempotent message）
↓
broadcast persisted message
```

V0.1 不需要：

- Redis Pub/Sub
- Kafka
- Message Queue
- 多節點 WebSocket
- typing indicator
- read receipt
- online presence

---

# 20. Chat History Pagination

不可設計成永久一次讀取全部聊天室訊息。

V0.1 使用 cursor pagination，例如：

```text
GET /api/v1/groups/{group_id}/messages?before={cursor}&limit=30
```

Cursor 需有 deterministic ordering，不要只依賴可能相同的 `created_at`；可使用 `(created_at, id)` 或等價穩定方案。

---

# 21. Delete Semantics

## 21.1 Meal

Meal 採 soft delete：

```text
deleted_at
```

Meal 被刪除後：

- 不再出現在個人飲食紀錄
- 已存在的聊天室上下文不 cascade 全刪
- Meal message 可顯示 tombstone，例如「此飲食紀錄已刪除」
- Reply chain 保留

## 21.2 Chat Message

訊息刪除後保留必要 thread 結構，顯示 tombstone，不應因 parent delete 導致整串回覆消失。

---

# 22. Today / 飲食歷史

V0.1 的 Today 頁面不顯示熱量。

範例：

```text
今天

已記錄 3 次

早餐
[photo]

午餐
[photo]

下午茶
[photo]

＋ 記錄飲食
```

歷史紀錄支援：

- 依日期查看 Meal
- 查看照片
- 查看時間 / 餐別 / 備註
- 編輯自己的 Meal
- 刪除自己的 Meal

「今天」依 `profiles.timezone` 計算。

---

# 23. Navigation

Mobile-first。

底部導航：

```text
今日
記錄
群組
我的
```

## 今日

- 今日已記錄 Meal
- 快速新增

## 記錄

- Create Meal workflow

## 群組

- 群組切換
- Chat
- Meal cards
- Reply / Reaction / GIF / Sticker

## 我的

- display name / avatar
- timezone
- 我的貼圖
- 群組相關設定

---

# 24. Tech Stack

## Frontend

```text
Next.js
TypeScript
Tailwind CSS
shadcn/ui
```

Mobile-first responsive Web。

V0.1 不做 native app。

## Backend

```text
Go
```

採 Modular Monolith。

建議 modules：

```text
/internal
/auth
/user
/profile
/group
/meal
/chat
/media
/sticker
/storage
```

## Database

```text
PostgreSQL
```

V0.1 不加入：

- MongoDB
- Redis
- Elasticsearch

---

# 25. Deployment

V0.1 運行於 Mac mini。

Docker Compose：

```text
web
api
postgres
fake-gcs
cloudflared
```

```text
Internet
↓
Cloudflare
↓
Cloudflare Tunnel
↓
Mac mini
├── Next.js
├── Go API
├── PostgreSQL
└── fake-gcs
```

規則：

- PostgreSQL 不直接暴露 Internet
- fake-gcs 不直接暴露 Internet
- Go API 不需直接開公網 port
- secrets 不 commit Git

---

# 26. Backup

至少每日備份：

- PostgreSQL
- fake-GCS persistent data

可先使用：

```text
pg_dump
+
volume / directory backup
```

備份至 External SSD / NAS。

---

# 27. Authorization

所有 protected operation 都由 Go 後端判斷，不可信任前端。

至少包含：

- Group message：必須是 group member
- Meal edit/delete：必須是 owner
- Meal read：owner 或有效 MealGroupShare 對應群組成員
- Meal image：跟隨 Meal authorization
- Reply / Reaction：必須能存取 parent message 所屬群組
- Sticker delete：必須是 sticker owner
- Chat media：依所屬 chat message / group scope 控制

不得使用 client 傳來的 `user_id` 決定 ownership。

---

# 28. API Draft

Base：

```text
/api/v1
```

## Auth

```text
POST /auth/register
POST /auth/login
POST /auth/logout
GET  /auth/me
```

## Profile

```text
GET   /me/profile
PATCH /me/profile
```

## Group

```text
POST /groups
GET  /groups
GET  /groups/{group_id}
GET  /groups/{group_id}/members

POST /groups/{group_id}/invites
POST /groups/join
```

## Meal

```text
POST   /meals
GET    /meals?date=YYYY-MM-DD
GET    /meals/{meal_id}
PATCH  /meals/{meal_id}
DELETE /meals/{meal_id}
```

## Meal Photo

```text
POST   /meals/{meal_id}/photos
GET    /meal-images/{image_id}
DELETE /meal-images/{image_id}
```

## Meal Share

```text
POST   /meals/{meal_id}/shares
DELETE /meals/{meal_id}/shares/{group_id}
```

## Chat

```text
GET  /groups/{group_id}/messages?before={cursor}&limit=30
POST /groups/{group_id}/messages
GET  /ws/groups/{group_id}
```

## Reaction

```text
POST   /messages/{message_id}/reactions
DELETE /messages/{message_id}/reactions/{reaction_type}
```

## Chat Media

```text
POST /chat-media
GET  /chat-media/{media_id}
```

## Sticker

```text
POST   /me/stickers
GET    /me/stickers
GET    /stickers/{sticker_id}/media
DELETE /me/stickers/{sticker_id}
```

API naming may be refined during implementation, but product/domain semantics above must be preserved.

---

# 29. Frontend Pages

建議：

```text
/login
/register
/join/{invite_code}
/onboarding

/today
/record

/groups
/groups/{group_id}

/profile
/profile/stickers
```

---

# 30. V0.1 開發 Phase

## Phase 1 — Infrastructure

- repository skeleton
- Docker Compose
- PostgreSQL
- fake-gcs
- Go API skeleton
- Next.js skeleton
- migration framework
- storage abstraction
- health endpoint
- `.env.example`
- development README

驗收：

```text
docker compose up
```

可啟動核心 local environment。

## Phase 2 — Authentication + Basic Profile

- users
- register/login/logout
- session middleware
- display_name
- avatar
- timezone
- protected frontend routes

## Phase 3 — Group + Invite

- create group
- join group
- invite link/code
- membership authorization
- owner invariant

## Phase 4 — Meal + Photo

- create/edit/delete Meal
- 1～4 photos
- fake-GCS upload
- image sanitization
- image proxy authorization
- photo/DB failure cleanup
- Today + history basic views

## Phase 5 — Chat Core

- text message
- WebSocket
- persisted-before-broadcast
- client_message_id idempotency
- cursor pagination
- reply
- reaction

## Phase 6 — Meal Sharing

- `meal_group_shares`
- Meal → ChatMessage reference
- realtime meal card
- shared meal/image authorization
- delete/tombstone behavior

## Phase 7 — Chat Media + GIF + Sticker

- chat image
- GIF upload/render
- user custom sticker
- GIF sticker
- sticker picker

## Phase 8 — UX Polish / MVP Validation

- mobile-first flow
- invitation onboarding
- fast meal record flow
- empty/loading/error states
- retry behavior
- basic usage instrumentation if explicitly selected

---

# 31. V0.1 Definition of Done

至少以下 end-to-end flow 完整成功：

```text
User A 建立群組
↓
分享 invite link
↓
User B 註冊並加入
↓
User A 拍照建立一筆 Meal
↓
Meal 照片成功保存
↓
Meal 自動分享至群組
↓
User B 不 reload 即時看到 Meal card
↓
User B Reaction / Reply
↓
User B 發送 GIF 或自訂 Sticker
↓
User A 即時看到互動
↓
User A 在 Today / History 仍可找到自己的 Meal
```

這個 Loop 成功且可實際每天使用，才算 V0.1 完成。

---

# 32. V0.1 明確不做

```text
BMR
TDEE
每日熱量目標
Calories tracking
Protein / Carbs / Fat
體重追蹤
AI 食物辨識
AI 熱量估算
營養資料庫
Apple Health
Health Connect
Push Notification
Native iOS
Native Android
公開社群
好友 / Follow 系統
排行榜
成就系統
付款 / 訂閱
GIPHY / Tenor 搜尋整合
圖片時間浮水印
Redis
Kafka
Microservices
Kubernetes
CDN
Signed URL
```

除非需求文件明確更新，coding agent 不得自行將上述功能加入 V0.1。

---

# 33. 後續 Roadmap

## V0.2 — Understand

目標：讓既有 Meal Record 產生更多飲食資訊。

可能包含：

```text
Meal photo
↓
食物辨識 / 使用者確認
↓
Calories / Macros
↓
每日攝取摘要
```

可研究：

- AI food recognition
- AI calorie estimation
- meal nutrition model
- nutrition source / confidence / user confirmation

## V0.3 — Control

目標：從「知道吃什麼」進入「知道應該吃多少」。

可能包含：

- 身高 / 體重 / 性別 / 年齡
- BMR
- TDEE
- daily calorie target
- weight history
- adaptive TDEE
- progress / recommendation

TDEE 公式與資料模型等到 V0.3 再正式定義，避免 V0.1 過早綁死。

---

# 34. Codex / Coding Agent 開發原則

1. V0.1-first，不自行加入 Roadmap 功能。
2. 採 Modular Monolith。
3. Domain logic 不放 HTTP Handler。
4. Storage 透過 interface。
5. 前端不可依賴 fake-GCS path / URL。
6. 圖片與 media 存取必須經 backend authorization。
7. MealMessage 只 reference MealRecord。
8. Comment 不獨立建模，使用 Reply Message。
9. 分享權限使用 `meal_group_shares`，不可只靠 ChatMessage 反推。
10. DB schema 全部使用 migration。
11. 核心 domain logic 需要 unit tests。
12. 關鍵 API / authorization 需要 integration tests。
13. WebSocket 需先 persistence 再 broadcast。
14. Chat send 必須考慮 idempotent retry。
15. 所有時間處理明確考慮 timezone。
16. 不 commit secrets。
17. Docker Compose 必須能建立完整 local environment。
18. 不做與當前 task 無關的大型 refactor。

---

# 35. Repository Structure

建議：

```text
chiban/
├── apps/
│   ├── web/
│   └── api/
├── migrations/
├── docker/
├── scripts/
├── docs/
│   └── MVP_SPEC.md
├── docker-compose.yml
├── .env.example
├── AGENTS.md
├── CLAUDE.md
└── README.md
```

Go 建議：

```text
apps/api/
├── cmd/api/
└── internal/
    ├── auth/
    ├── user/
    ├── profile/
    ├── group/
    ├── meal/
    ├── chat/
    ├── media/
    ├── sticker/
    └── storage/
```

---

# 36. 第一個 Coding Agent 任務

建議第一個任務仍只建立基礎設施：

> 根據 `docs/MVP_SPEC.md` 建立 monorepo skeleton，完成 Docker Compose、PostgreSQL、fake-gcs-server、Go API 與 Next.js 基礎專案。加入 health endpoint、database migration framework、storage abstraction、`.env.example` 與 README 開發啟動說明。此階段不要實作產品功能。

接著嚴格依 Phase 逐步實作與驗收。

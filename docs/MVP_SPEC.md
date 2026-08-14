# 共同飲控系統 MVP 需求與技術規格

## 1. 文件目的

本文件用於定義「共同飲控系統」第一版 MVP 的產品需求、系統邊界、資料模型、API、權限規則與開發順序，作為後續 Codex 開發的主要依據。

MVP 的首要目標不是做一套完整的營養管理平台，而是驗證以下核心假設：

> 一群朋友共同記錄飲食，透過聊天室互相回覆、鼓勵、吐槽與監督，是否能提升飲食記錄與飲食控制的持續性。

---

# 2. 產品定位

共同飲控系統是一個小型封閉社群的飲食紀錄與共同監督工具。

每位使用者可以：

1. 建立帳號與個人身體資料
2. 計算預估 BMR / TDEE / 每日熱量目標
3. 記錄每日每一餐
4. 上傳餐點照片
5. 將餐點紀錄自動分享至群組聊天室
6. 對朋友的餐點紀錄進行 Reaction / Reply
7. 使用一般聊天室進行文字聊天
8. 查看自己每天的飲食紀錄、熱量與剩餘額度

核心概念：

> Record once, share automatically.

飲食資料只建立一次，聊天室只引用 MealRecord，不複製飲食資料。

---

# 3. MVP 核心使用流程

## 3.1 首次使用

```text
註冊
↓
登入
↓
建立 Profile
↓
輸入身高 / 體重 / 性別 / 年齡
↓
選擇活動程度
↓
選擇飲食目標
↓
系統計算 BMR / TDEE / 每日熱量目標
↓
加入或建立群組
↓
進入首頁
```

## 3.2 每日飲食紀錄

```text
點擊「記錄」
↓
拍攝 / 選擇餐點照片
↓
輸入餐別
↓
輸入餐點描述
↓
輸入或估算熱量
↓
設定時間
↓
選擇是否分享至群組
↓
建立 MealRecord
↓
照片存入 Object Storage
↓
自動建立 Meal ChatMessage
↓
群組成員即時看到
```

## 3.3 群組互動

```text
朋友看到 Meal Message
↓
Reaction
or
Reply
or
一般文字聊天
↓
原使用者收到聊天室內容
```

---

# 4. MVP 功能範圍

## 4.1 Authentication

必要功能：

- 註冊
- 登入
- 登出
- Session 驗證
- Password hash
- API Authentication middleware

第一版登入方式：

- Email
- Password

暫不實作：

- Google OAuth
- Apple Login
- LINE Login
- MFA

---

# 5. 使用者 Profile

## 5.1 Profile 欄位

```text
display_name
avatar
gender
birthday
height_cm
current_weight_kg
```

## 5.2 Goal

```text
goal_type

lose_weight
maintain
gain_weight
```

其他欄位：

```text
target_weight_kg
activity_level
daily_calorie_target
```

---

# 6. BMR / TDEE

第一版使用 Mifflin-St Jeor Equation。

## 男性

```text
BMR =
10 × weight_kg
+ 6.25 × height_cm
- 5 × age
+ 5
```

## 女性

```text
BMR =
10 × weight_kg
+ 6.25 × height_cm
- 5 × age
- 161
```

## Activity Factor

```text
sedentary          1.2
light              1.375
moderate           1.55
active             1.725
very_active        1.9
```

## TDEE

```text
TDEE = BMR × activity_factor
```

## Daily Calorie Target

第一版：

```text
maintain:
TDEE

lose_weight:
TDEE × 0.85

gain_weight:
TDEE × 1.10
```

系統 UI 必須使用：

> 預估 TDEE

不可將 TDEE 顯示為精確消耗。

---

# 7. TDEE Calculation History

不要只在 profile 存最終 TDEE。

需要保留計算歷史。

```text
tdee_calculations
-----------------
id
user_id

weight_kg
height_cm
age
gender

activity_level
activity_factor

formula
formula_version

bmr
tdee

goal_type
goal_adjustment
daily_calorie_target

calculated_at
```

第一版：

```text
formula = mifflin_st_jeor
formula_version = 1
```

當使用者修改：

- 體重
- 身高
- 活動程度
- 目標

重新產生 calculation record。

---

# 8. Weight Record

需要獨立保留體重歷史。

```text
weight_records
--------------
id
user_id
weight_kg
recorded_at
created_at
```

Profile 的 current_weight 可以作為目前快取值。

---

# 9. Group

系統為封閉群組，而不是公開社群。

```text
groups
------
id
name
owner_id
created_at
updated_at
```

群組成員：

```text
group_members
-------------
group_id
user_id
role
joined_at
```

Role 第一版：

```text
owner
member
```

MVP 支援：

- 建立群組
- 產生邀請碼
- 使用邀請碼加入群組
- 查看群組成員

第一版可以限制：

> 一位 User 可以加入多個 Group。

---

# 10. Meal Record

MealRecord 是產品最重要的 Primary Data。

```text
meal_records
------------
id
user_id

meal_type
eaten_at

description

estimated_calories
protein_g
carbs_g
fat_g

created_at
updated_at
deleted_at
```

Meal Type：

```text
breakfast
lunch
dinner
snack
other
```

MVP nutrition：

必填：

```text
meal_type
eaten_at
```

選填：

```text
description
estimated_calories
protein_g
carbs_g
fat_g
```

第一版不要求 AI 自動辨識。

---

# 11. Meal Photo

每筆 Meal 可以有多張圖片。

第一版限制建議：

```text
1 ~ 4 images / meal
```

Table：

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

---

# 12. Object Storage

目前 MVP 使用：

> fake-gcs-server

運行於 Mac mini Docker 環境。

Bucket：

```text
meal-images
```

推薦 Object Path：

```text
users/{user_id}/meals/{year}/{month}/{uuid}.webp
```

例如：

```text
users/01ABC/meals/2026/08/550e8400.webp
```

前端不可依賴：

```text
bucket
object_name
fake-gcs URL
```

Frontend 只認：

```text
image_id
```

---

# 13. Image Authorization

第一版圖片讀取：

```text
Browser
↓
GET /api/meal-images/{image_id}
↓
Go API
↓
Authentication
↓
Authorization
↓
fake GCS
↓
Go stream image
↓
Browser
```

Go 負責圖片存取權限。

不可直接公開 fake GCS bucket。

不可讓前端直接指定 object path，例如：

```text
禁止：

GET /images?path=xxx
```

必須：

```text
GET /api/meal-images/{image_id}
```

Server 自行根據 image_id 查詢：

```text
MealPhoto
↓
MealRecord
↓
User / Group authorization
↓
bucket + object_name
```

未來可以改：

```text
Go authorization
↓
Signed URL
↓
Browser → GCS
```

但不屬於 MVP。

---

# 14. 飲食分享機制

建立 MealRecord 時：

```text
share_to_group_ids[]
```

如果使用者選擇分享至 Group：

系統建立 ChatMessage：

```text
message_type = meal
meal_record_id = {meal_id}
```

ChatMessage 只引用 MealRecord。

禁止複製：

```text
meal description
calories
photo URL
```

到 ChatMessage。

避免資料不同步。

---

# 15. Chat Room

每個 Group 對應一個聊天室。

第一版 Message Type：

```text
text
meal
image
system
```

Table：

```text
chat_messages
-------------
id
group_id
user_id

message_type

content
meal_record_id

reply_to_message_id

created_at
updated_at
deleted_at
```

---

# 16. Reply

所有聊天室訊息都可以被 Reply。

使用：

```text
reply_to_message_id
```

例如：

```text
Meal Message
↑
Reply Message
```

不另外建立 Comment table。

核心原則：

> Comment 與 Chat 不拆成兩套系統。

針對飲食的評論本質為：

```text
Reply to Meal ChatMessage
```

---

# 17. Reaction

Table：

```text
message_reactions
-----------------
message_id
user_id
reaction_type
created_at
```

Unique constraint：

```text
message_id
user_id
reaction_type
```

第一版 Reaction：

```text
❤️
😂
🔥
👏
👀
```

後續可增加飲控專屬 Reaction。

---

# 18. Realtime Chat

第一版使用 Go WebSocket。

架構：

```text
Browser
↓
WebSocket
↓
Go API
↓
Chat Hub
↓
Group connection pool
```

一個 Group 對應一個 broadcast room。

流程：

```text
Client A
↓
send message
↓
Go
↓
DB INSERT
↓
broadcast
↓
Client B / C / D
```

MVP 不需要：

- Redis Pub/Sub
- Kafka
- Message Queue
- 多節點 websocket
- typing indicator
- read receipt
- online presence

---

# 19. 今日頁面

首頁之一：

```text
今日目標
2300 kcal

已攝取
1520 kcal

剩餘
780 kcal
```

並列出：

```text
早餐
午餐
晚餐
點心
```

計算：

```text
daily_consumed =
SUM(meal_records.estimated_calories)
WHERE eaten_at = today
```

```text
remaining =
daily_calorie_target - daily_consumed
```

---

# 20. 歷史紀錄

第一版支援：

- 按日期查看 Meal
- 查看照片
- 查看熱量
- 編輯 Meal
- 刪除自己的 Meal

不需要：

- Weekly chart
- Monthly report
- AI insight

---

# 21. Navigation

Mobile-first。

底部導航：

```text
今日
記錄
群組
我的
```

其中：

## 今日

查看：

- 熱量目標
- 已攝取
- 剩餘
- 今日 Meal

## 記錄

Create Meal workflow。

## 群組

聊天室 / 群組切換。

## 我的

Profile / Goal / TDEE / Weight。

---

# 22. Tech Stack

## Frontend

```text
Next.js
TypeScript
Tailwind CSS
shadcn/ui
```

Mobile-first responsive Web。

第一版不做 native app。

---

# 23. Backend

```text
Go
```

架構：

> Modular Monolith

建議 module：

```text
/internal

/auth
/user
/profile
/goal
/tdee
/weight
/group
/meal
/chat
/storage
```

不要建立 microservices。

---

# 24. Database

```text
PostgreSQL
```

第一版所有 relational data 存 PostgreSQL。

不要加入：

- MongoDB
- Redis
- Elasticsearch

---

# 25. Storage

```text
fake-gcs-server
```

Docker container。

實作 storage abstraction：

```go
type ObjectStorage interface {
    Upload(...)
    Delete(...)
    Open(...)
}
```

第一版：

```text
FakeGCSStorage
```

未來：

```text
GoogleCloudStorage
```

不影響 domain layer。

---

# 26. Deployment

運行環境：

> Mac mini

Docker Compose：

```text
web
api
postgres
fake-gcs
cloudflared
```

概念：

```text
Internet
   │
Cloudflare
   │
Cloudflare Tunnel
   │
Mac mini
   │
   ├── Next.js
   ├── Go API
   ├── PostgreSQL
   └── fake-gcs
```

PostgreSQL 不暴露至 Internet。

fake-gcs 不暴露至 Internet。

Go API 不直接開 public port。

---

# 27. Backup

MVP 也必須做資料備份。

至少備份：

```text
PostgreSQL
fake GCS data
```

建議每日：

```text
pg_dump
+
fake-gcs volume backup
```

備份至：

- External SSD
or
- NAS

異地備份可以後續加入。

---

# 28. Security

## Password

不可存 plaintext。

使用：

```text
Argon2id
```

或：

```text
bcrypt
```

## Session

推薦：

```text
HttpOnly
Secure
SameSite
```

Cookie。

## Authorization

所有 API 不可信任 frontend。

例如：

```text
GET /groups/{group_id}/messages
```

Server 必須確認：

```text
group_members
WHERE group_id = ?
AND user_id = current_user
```

圖片同樣需要 server authorization。

---

# 29. 建議資料關係

```text
User
│
├── Profile
├── Goal
├── WeightRecord
├── TDEECalculation
└── MealRecord
      │
      └── MealPhoto


User
│
└── GroupMember
      │
      └── Group
            │
            └── ChatMessage
                  │
                  ├── MealRecord
                  ├── Reply
                  └── Reaction
```

---

# 30. API Draft

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

## Goal / TDEE

```text
GET  /me/goal
PUT  /me/goal

GET  /me/tdee
POST /me/tdee/recalculate
```

## Weight

```text
GET  /me/weights
POST /me/weights
```

## Group

```text
POST /groups
GET  /groups
GET  /groups/{group_id}

POST /groups/{group_id}/invite
POST /groups/join

GET  /groups/{group_id}/members
```

## Meal

```text
POST   /meals
GET    /meals
GET    /meals/{meal_id}
PATCH  /meals/{meal_id}
DELETE /meals/{meal_id}
```

Query：

```text
GET /meals?date=2026-08-14
```

## Meal Photo

```text
POST   /meals/{meal_id}/photos
GET    /meal-images/{image_id}
DELETE /meal-images/{image_id}
```

## Chat

```text
GET  /groups/{group_id}/messages
POST /groups/{group_id}/messages
```

WebSocket：

```text
GET /ws/groups/{group_id}
```

## Reaction

```text
POST   /messages/{message_id}/reactions
DELETE /messages/{message_id}/reactions/{reaction_type}
```

---

# 31. Frontend Pages

建議：

```text
/login
/register
/onboarding

/today
/record

/groups
/groups/{group_id}

/profile
/profile/goal
/profile/weight
```

---

# 32. MVP 開發 Phase

## Phase 1 — Infrastructure

完成：

- Docker Compose
- PostgreSQL
- fake-gcs
- Go API skeleton
- Next.js skeleton
- Cloudflare Tunnel local config
- migration system

驗收：

```text
docker compose up
```

可以啟動所有核心服務。

---

## Phase 2 — Authentication

完成：

- users table
- register
- login
- logout
- session middleware
- protected routes

驗收：

未登入使用者不能存取：

```text
/today
/groups
/meals
```

---

## Phase 3 — Profile / TDEE

完成：

- onboarding
- profile
- goal
- TDEE calculation
- weight record

驗收：

使用者輸入：

```text
gender
birthday
height
weight
activity level
goal
```

可以得到：

```text
BMR
TDEE
Daily Calorie Target
```

---

## Phase 4 — Meal Record

完成：

- create meal
- edit meal
- delete meal
- list today's meals
- upload images
- image proxy authorization

驗收：

使用者可以：

```text
新增午餐
上傳照片
重新整理頁面
仍可看到照片與 Meal
```

---

## Phase 5 — Group

完成：

- create group
- invite code
- join group
- member list

驗收：

兩個不同帳號可以加入同一個群組。

---

## Phase 6 — Chat

完成：

- text message
- meal message
- reply
- reaction
- WebSocket realtime

驗收：

User A 發訊息後：

User B 不 reload 頁面即可看到。

---

## Phase 7 — Meal Social Sharing

完成：

```text
Create Meal
↓
share_to_group
↓
automatic MealMessage
↓
other users see
↓
Reply / Reaction
```

這是 MVP 最重要的整合驗收。

---

## Phase 8 — Today UX

完成：

```text
Daily Calorie Target
Consumed
Remaining
Meals
```

驗收：

新增 Meal 後數值立即更新。

---

# 33. MVP Definition of Done

只有以下完整流程全部成功，才算 MVP 完成：

```text
User A 註冊
↓
設定 Profile
↓
取得 TDEE
↓
建立 Group
↓
User B 加入
↓
User A 記錄 Meal
↓
上傳照片
↓
Meal 自動出現在 Group Chat
↓
User B 即時看到
↓
User B Reaction
↓
User B Reply
↓
User A 看到 Reply
↓
User A Today page 熱量同步更新
```

---

# 34. 第一版明確不做

不要讓 Scope 擴張。

MVP 不做：

```text
AI 食物辨識
AI 熱量估計
Apple Health
Google Health Connect
Push Notification
Native iOS
Native Android
公開社群
好友系統
排行榜
成就系統
營養師功能
付款
訂閱
完整營養資料庫
Redis
Kafka
Microservices
Kubernetes
CDN
Signed URL
```

---

# 35. 後續可擴充方向

MVP 驗證成功後：

## AI Meal Analysis

```text
Meal Photo
↓
Vision Model
↓
Food Recognition
↓
Calories / Macro Estimate
↓
User Confirm
```

## Adaptive TDEE

```text
Calorie Intake
+
Weight Trend
+
Time
↓
Estimate Real TDEE
```

## Health Integration

```text
Apple Health
Health Connect
```

## Notifications

例如：

```text
朋友記錄了一餐
有人回覆你的飲食
今天尚未記錄午餐
```

---

# 36. Codex 開發原則

Codex 開發時必須遵守：

1. MVP-first，不自行加入未定義功能。
2. 採 Modular Monolith。
3. Domain logic 不放在 HTTP Handler。
4. Storage 必須透過 interface。
5. 前端不可依賴 fake GCS object path。
6. 圖片存取必須經 backend authorization。
7. ChatMessage 只 reference MealRecord，不複製 Meal data。
8. Comment 不獨立建模，使用 Reply Message。
9. TDEE 必須保留 calculation history。
10. 所有 database schema 使用 migration 管理。
11. API 需有基礎 integration tests。
12. 核心 domain logic 需有 unit tests。
13. 不加入 Redis / Kafka / Microservices，除非規格更新。
14. secrets 不 commit 至 Git。
15. Docker Compose 必須能建立完整 local environment。

---

# 37. 建議 Repository Structure

```text
diet-control/
│
├── apps/
│   ├── web/
│   │   └── Next.js
│   │
│   └── api/
│       └── Go
│
├── migrations/
│
├── docker/
│
├── scripts/
│
├── data/
│   ├── postgres/
│   └── fake-gcs/
│
├── docs/
│   └── MVP_SPEC.md
│
├── docker-compose.yml
├── .env.example
└── README.md
```

Go：

```text
apps/api/

cmd/
  api/

internal/
  auth/
  user/
  profile/
  goal/
  tdee/
  weight/
  group/
  meal/
  chat/
  storage/

pkg/
```

---

# 38. 第一個 Codex 任務建議

不要直接一次要求 Codex 完成全部功能。

第一個任務：

> 根據 docs/MVP_SPEC.md 建立 monorepo skeleton，完成 Docker Compose、PostgreSQL、fake-gcs-server、Go API 與 Next.js 基礎專案。加入 health endpoint、database migration framework、storage abstraction 與 README 開發啟動說明。此階段不要實作產品功能。

第二個任務：

> 實作 Authentication + User/Profile schema。

第三個任務：

> 實作 Goal / Weight / TDEE。

依 Phase 逐步完成。

---

# 39. MVP 核心產品 Loop

最終所有技術與 UX 都服務這個 Loop：

```text
吃東西
↓
記錄
↓
拍照
↓
系統計算今日熱量
↓
分享至群組
↓
朋友 Reaction / Reply
↓
形成 Accountability
↓
影響下一餐選擇
↓
再次記錄
```

如果 MVP 無法驗證這個 Loop，就不應優先加入其他進階功能。

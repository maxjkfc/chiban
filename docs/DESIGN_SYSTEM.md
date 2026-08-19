# 吃伴 Design System v0.2 — 貼紙拼貼

一本吃飯日記，不是一張表單。內容是照片，介面是貼著它的紙。

系統只有一層真實來源：[`apps/web/app/globals.css`](../apps/web/app/globals.css) 的 CSS 變數。元件一律吃 token，不寫死顏色與尺寸。

## v0.1 → v0.2 改了什麼

v0.1 是暖色調的表單：四頁同一個骨架（`h1` + 直排欄位 + 全寬按鈕）、照片被排成四欄約 80px 的方格、全站一個圓角一層陰影。v0.2 換掉的是版型與層級，不是換一組顏色：

| | v0.1 | v0.2 |
|---|---|---|
| 主色 | `oklch(0.58 0.19 32)`，白字 4.29:1（未過 AA） | `--primary` 0.55 給底（白字 4.9:1）、`--primary-ink` 0.5 給紙上的字（4.6:1） |
| 圓角 | `--radius: 1rem` 一階套滿 | 三階：卡片 4px、面板 16px、按鈕全圓 |
| 照片 | `grid-cols-4` 約 80px | 單張 ≥ 136px，多張 ≥ 118px |
| 底色 | 純奶油 | 奶油 + 7px 點狀紙紋 |
| 標題 | 與內文同一套 sans | serif（`--font-heading`）對比 sans 內文 |
| 輔助文字 | `oklch(0.52 0.035 60)` | `oklch(0.48 0.03 60)`，紙上 4.9:1 |

## 設計原則

1. **照片是內容，紙是介面。** 任何一張餐點照片不得小於 118px。縮圖格線是這次拆掉的東西。
2. **一個主色講一件事。** 珊瑚只給「下一步」（主要按鈕、目前分頁、已選狀態）。滿版的橘會讓照片失焦。
3. **圓角分三階。** 4px 讀成相紙、16px 讀成面板、全圓讀成可按。一階套滿的結果是一致但沒有輕重。
4. **歪斜只給卡片，±1.5° 以內。** 按鈕、輸入框、導覽列永遠是正的——歪掉的按鈕會讓人以為壞了。
5. **一張卡一條膠帶，只貼頂邊。** 膠帶是節奏，不是裝飾預算。
6. **拇指優先。** 可點擊元素 44px。視覺上更小的（反應膠囊）用透明 padding 把觸控範圍撐到 44px。
7. **活潑不能吃掉可讀性。** 紙上的文字最低 4.5:1，最小字級 `text-xs`。

## Design tokens

### 顏色（語意層）

| Token | 值 | 用途 |
|---|---|---|
| `--background` | `oklch(0.965 0.014 88)` | 紙。`body` 另外疊 `--paper-grain` 的 7px 點狀紋理 |
| `--foreground` | `oklch(0.26 0.02 60)` | 墨。主文字，不用純黑 |
| `--card` / `--popover` | 白 | 拍立得白邊、面板 |
| `--primary` | `oklch(0.55 0.17 32)` | 主要行動的**底色**。白字 4.9:1 |
| `--primary-foreground` | 近白 | 主色上的文字 |
| `--primary-ink` | `oklch(0.5 0.17 32)` | 紙上的珊瑚**文字**（4.6:1）。目前分頁、連結、強調數字 |
| `--muted-foreground` | `oklch(0.48 0.03 60)` | 輔助文字。紙上 4.9:1 |
| `--secondary` | `oklch(0.93 0.09 95)` 奶油黃 | 次要 chip、檔案選擇鈕 |
| `--accent` | `oklch(0.91 0.09 165)` 薄荷 | 成功、標籤（管理者） |
| `--destructive` | `oklch(0.52 0.2 20)` | 錯誤與破壞性操作 |
| `--border` / `--input` / `--ring` | 暖灰 / 珊瑚 | 描邊與 focus ring |

**`--primary` 與 `--primary-ink` 不可互換。** 底色拿去當文字會掉到 3.8:1；文字色拿去當底色會讓白字過暗。哪個都不對。

### 顏色（裝飾層）

`--tape-1` … `--tape-4`（`oklch(0.62 0.14 30 / 95 / 200 / 330)`）是膠帶。同一個 L 與 C，只換色相，所以四條放在一起不會有一條特別跳。

`--brand` `--mango` `--mint` `--sky` `--berry` 用在「需要區分但沒有語意」的地方：成員頭像底色、圖表。

**膠帶與裝飾色都不承載文字，也不承載語意。** 錯誤永遠是 `destructive`，成功永遠是 `accent`。

### 圓角

| Token | 值 | 用在 |
|---|---|---|
| `--radius-sm` | 4px | 拍立得卡、卡上的照片 |
| `--radius-md` | 12px | 縮圖、貼圖格 |
| `--radius-lg` | 16px | 面板（`card-surface`）、`Input` 方框、日期選擇 |
| `--radius-xl` | 22px | 底部面板頂角 |
| `--radius-2xl` | 26px | 大面板 |
| `--radius-3xl` | 34px | 裝置外框（僅原型） |
| 按鈕 | `rounded-full` | 所有按鈕，不吃上面任何一階 |

### 字

| Token | 值 |
|---|---|
| `--font-sans` | Geist → PingFang TC → Noto Sans TC → Microsoft JhengHei |
| `--font-heading` | Georgia → Songti TC → Noto Serif TC → Source Han Serif TC |

兩套都是系統字，**刻意不載 CJK webfont**：一個中文字重就算 subset 過還是好幾 MB，行動優先的 app 不值得。Georgia 排在標題最前面是為了拉丁字與數字，它沒有中文字，所以中文會往後掉到 Songti / Noto Serif（macOS、iOS、Android 各自都有）。

| 用途 | 尺寸 |
|---|---|
| 分頁標題（今日 / 記錄 / 群組 / 我的） | `h1` 1.9rem / `font-black` / serif（全域，不要再蓋掉） |
| 次級頁標題（有返回鍵：這一餐、我的貼圖） | `h1.page-title-sub` 1.5rem |
| 緊湊列標題（有副標：群組聊天） | `h1.page-title-bar` 1.125rem |
| `h2`、餐別、群組名 | `font-heading font-black`，0.95–1.05rem |
| 內文 | `text-sm` |
| 輔助 | `text-xs`（最小；不要更小） |

返回鍵進來的畫面不是分頁，標題不該吼得一樣大。所以有兩個具名的例外，**都還是 `h1`**（文件大綱要正確）。除了這兩個 utility，不要自己在 `h1` 上加字級——`text-2xl` 這種寫法會在下次改全域字級時默默脫隊。

### 其他

`shadow-pop` = `0 10px 18px -14px var(--pop-shadow)`。唯一的陰影階層，暖色投影不是灰色。

間距：頁面 `px-5 pt-7 pb-5`、區塊之間 `gap-4`、卡片列表 `gap-5`（歪斜需要餘裕）、欄位群 `gap-4`、label 與控制項 `gap-1.5`。

## Utilities

| Utility | 等於 | 用在 |
|---|---|---|
| `polaroid` | `bg-card border rounded-sm border p-2 pb-3 shadow-pop` | 每一餐、聊天室的照片與飲食紀錄、頭像 |
| `tape` | `absolute h-5 border-x border-dashed`（父層要 `relative`） | 卡片頂邊 |
| `ruled` | 27px 一條的橫線背景 | `Input variant="ruled"` |
| `card-surface` | `bg-card rounded-lg border p-4 shadow-pop` | 設定、分享、成員這種列表面板 |

`polaroid` 的 padding 是不對稱的：下緣比較寬，因為那裡是寫字的地方——跟真的拍立得一樣。

## 元件

### Tape — `components/tape.tsx`

```tsx
<div className={cn("polaroid relative", tiltClass(index))}>
  <Tape tone={tapeTone(index)} className={tapePlacement(index)} />
  …
</div>
```

`tiltClass` / `tapeTone` / `tapePlacement` 都由**索引**決定，不是亂數：一個列表如果每次重繪都重新抽角度，看起來會像壞掉而不是手作。角度限制在 ±1.5°。

`Tape` 是 `aria-hidden` 且 `pointer-events-none`——它壓在卡片上，會吃點擊的膠帶等於在每一餐頂端放一條死區。

### Button — `components/ui/button.tsx`

| Variant | 用在 |
|---|---|
| `default` | 頁面主要行動（貼上去、建立、儲存、登入）。一頁最多一個 |
| `outline` | 次要行動（登出、撤銷連結、圖示按鈕） |
| `secondary` | 已展開／已啟用的工具鈕 |
| `ghost` | 工具列 |
| `destructive` | 破壞性操作，淡紅底、紅字 |
| `link` | 行內文字連結 |

尺寸：`default` 44px、`lg` 52px、`icon` 44px 方形。`sm` 28px 與 `xs` 24px 只留給密集列表內的動作，**不要**用在主要流程上。

`loading` 只負責等待狀態；`disabled` 留給「條件未滿足」（例如還沒選照片）。兩者可並存。

### Input — `components/ui/input.tsx`

兩個 variant：

- `box`（預設）— 44px 高、`rounded-lg`、白底。給真正是表單的地方：帳密、日期、搜尋。
- `ruled` — 一條橫線，沒有框。給該用手寫的地方：餐點備註、暱稱、新群組名稱。focus 時是外框 outline，不動那條線。

錯誤狀態用 `aria-invalid`，不要自己加 class。

### Message — `components/ui/message.tsx`

`error`（淡紅膠囊、`role="alert"`）／`success`（薄荷）／`info`（純灰字）。未變更。

### BottomNav — `components/bottom-nav.tsx`

四個固定分頁：今日 / 記錄 / 群組 / 我的。目前分頁是**珊瑚色文字 + 底下一條短線**，不是填滿的膠囊——膠囊會讓四個分頁都讀成按鈕，但一次只有一個東西要按。

顏色不是唯一線索：圖示放大、文字加粗、短線出現，三個一起。

### StickerPicker — `components/sticker-picker.tsx`

聊天室的貼圖，三個部分：

1. **快捷列** — 輸入框是空的時候，4 張貼圖躺在鍵盤上方，一點就送。開始打字就收起來（否則整段對話都要付 76px 的高度）。
2. **選擇盤** — Radix `Dialog` 的底部面板，不再插在流裡把對話往上推。3 欄約 111px（v0.1 是 4 欄約 80px，GIF 在那個尺寸認不出來）。右上「加一張」就地上傳，不用離開聊天室。
3. **編輯快捷列** — 4 個明確的格子，點貼圖填進去、再點一次拿掉，順序就是點的順序。**沒有拖曳、沒有長按**：那都是要教的手勢。

哪 4 張是**使用者自己選的**，存在 `user_stickers.pin_order`（1–4，partial unique index 保證一格一張）。沒設過就退回最新的 4 張——不能讓人第一次用貼圖前得先去設定。

貼圖庫在**進聊天室時就載入**，不再是第一次打開才載：快捷列沒有它畫不出來，而那份清單只有 id 與 type。

「載入失敗」與「你沒有貼圖」永遠不會長一樣——兩者的下一步完全不同。

### 訊息泡泡 — `components/chat-room.tsx`

**紙感只給媒體。** 照片、貼圖、飲食紀錄拿到 `polaroid` 外框；純文字維持安靜的白泡泡（自己的是珊瑚底），只歪 0.4°。長對話才讀得下去——這是這個方向最大的風險，刻意這樣壓住。

反應是壓在訊息上的小貼紙：視覺 32px，用 `after:absolute after:-inset-x-1 after:-inset-y-1.5` 把觸控範圍撐到 44px。**不要**為了 44px 把膠囊本身畫大。

## Do / Don't

| ✅ Do | ❌ Don't |
|---|---|
| 用語意 token（`bg-primary`、`text-primary-ink`） | 寫死 hex、oklch、`bg-[#ff6b35]` |
| 紙上的珊瑚字用 `text-primary-ink` | 用 `text-primary`（底色）當文字 |
| `<h1>標題</h1>`、或 `page-title-sub` / `page-title-bar` | `<h1 className="text-2xl font-semibold">` — 自己蓋全域字級 |
| 卡片用 `tiltClass(index)` | 用 `Math.random()` 決定角度 |
| 錯誤用 `<Message tone="error">` | 自己拼 `text-destructive text-sm` |
| 照片至少 118px | 為了塞更多筆而縮成縮圖格 |
| 反應膠囊靠透明 padding 達到 44px | 把膠囊畫成 44px |

## 目前缺口

- **無 dark mode 切換器，而且暫時不做。** `.dark` 的 token 是完整的，但紙紋在深色底下會讀成假的材質。要加切換器的人得先解掉那件事。
- 無 Badge / Toast——等實際使用點出現再建。
- 動效仍只有按壓、spinner 與貼圖送出的縮放。照片上傳、訊息進場的動畫還沒做。
- `Button` 的 `sm` / `xs` 低於 44px。目前只用在密集列表，但沒有 lint 擋住誤用。

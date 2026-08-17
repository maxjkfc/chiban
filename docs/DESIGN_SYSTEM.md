# 吃伴 Design System v0.1

活潑、暖色、行動優先。目標使用者是年輕人，介面要像社群 app 而不是健康管理表單。

系統只有一層真實來源：[`apps/web/app/globals.css`](../apps/web/app/globals.css) 的 CSS 變數。元件一律吃 token，不寫死顏色與尺寸。

## 設計原則

1. **一個主色講一件事**：珊瑚橘只給「下一步」用（主要按鈕、目前分頁）。滿版的橘會讓照片失焦，而照片才是主角。
2. **奶油底 + 白卡片**：內容區塊靠白色卡片浮在暖底色上分層，而不是靠更多邊框。
3. **圓的**：`--radius: 1rem`，按鈕全圓角。這是「年輕」感的主要來源，比顏色更明顯。
4. **拇指優先**：可點擊元素最小 44px（`h-11`）。
5. **活潑不能吃掉可讀性**：主色亮度壓在 L 0.58，白字對比 4.5:1 過 WCAG AA。

## Design tokens

### 顏色（語意層）

| Token | Light | 用途 |
|---|---|---|
| `--background` | `oklch(0.975 0.018 78)` 暖奶油 | 頁面底色，取代純白 |
| `--card` / `--popover` | 白 | 浮在底色上的內容區塊 |
| `--foreground` | `oklch(0.25 0.03 55)` 暖黑 | 主文字，不用純黑 |
| `--primary` | `oklch(0.58 0.19 32)` 珊瑚橘 | 主要行動、目前分頁 |
| `--primary-foreground` | 近白 | 主色上的文字 |
| `--secondary` | `oklch(0.93 0.09 95)` 奶油黃 | 次要 chip、檔案選擇鈕 |
| `--accent` | `oklch(0.91 0.09 165)` 薄荷綠 | 成功、標籤（管理者） |
| `--muted` / `--muted-foreground` | 暖灰 | 輔助說明文字 |
| `--destructive` | `oklch(0.55 0.21 18)` | 錯誤 |
| `--border` / `--input` / `--ring` | 暖灰 / 珊瑚 | 描邊與 focus ring |

### 顏色（裝飾層）

`--brand` `--mango` `--mint` `--sky` `--berry`，對應 Tailwind 的 `bg-brand` `text-mint` …，也接到 `--chart-1..5`。

用途是「需要區分但沒有語意」的地方：群組色票、成員頭像底色、貼圖分類。**不要**拿來當語意色（錯誤永遠是 `destructive`，成功永遠是 `accent`）。

### 尺寸與其他

| Token | 值 | 備註 |
|---|---|---|
| `--radius` | `1rem` | `rounded-lg` = 1rem，`rounded-2xl` = 1.8rem |
| `shadow-pop` | `0 8px 20px -10px var(--pop-shadow)` | 唯一的陰影階層；暖色投影，不是灰色 |
| 間距 | 頁面 `p-6`、區塊之間 `gap-6`、欄位群 `gap-4`、label 與控制項 `gap-2` | |
| 字級 | `h1` 1.75rem/extrabold（全域，不要再加 `text-2xl`）、`h2` bold、內文 `text-sm`、輔助 `text-xs` | |

Dark mode 的 token 已備齊（`.dark`），但目前沒有切換器，實際不會觸發。

## 元件

### Button — `components/ui/button.tsx`

| Variant | 用在 |
|---|---|
| `default` | 頁面主要行動（發布、建立、儲存、登入）。一頁最多一個 |
| `outline` | 次要行動（登出、撤銷連結） |
| `ghost` | 工具列、圖示按鈕 |
| `destructive` | 破壞性操作，淡紅底、紅字 |
| `link` | 行內文字連結 |

| Size | 高度 | 用在 |
|---|---|---|
| `default` | 44px | 幾乎全部 |
| `lg` | 52px | 單一 CTA 的落地頁 |
| `sm` / `xs` | 28 / 24px | 密集列表內的動作 |
| `icon*` | 正方形 | 純圖示 |

| State | 行為 |
|---|---|
| Hover | 主色 90% |
| Active | 下移 0.5px + 縮 98%，給實體按壓感 |
| Focus | 3px 珊瑚 ring |
| `loading` | 顯示旋轉圖示、`aria-busy`、自動 disabled |
| Disabled | 50% 透明、不可點 |

```tsx
<Button loading={submitting}>{submitting ? "儲存中…" : "儲存"}</Button>
```

`loading` 只負責等待狀態；`disabled` 留給「條件未滿足」（例如還沒選照片）。兩者可並存。
`asChild` 時不會插入 spinner——Slot 只能有一個子節點。

### Input — `components/ui/input.tsx`

44px 高、`rounded-2xl`、白底浮在奶油底色上。`type="file"` 的選擇鈕會渲染成奶油黃 pill。

錯誤狀態用 `aria-invalid`（紅框 + 紅 ring），不要自己加 class。

### Message — `components/ui/message.tsx`

表單與頁面的即時回饋，取代散落各處的 `<p className="text-destructive text-sm">`。

| Tone | 外觀 | role |
|---|---|---|
| `error` | 淡紅底圓角膠囊 | `alert`（立即朗讀） |
| `success` | 薄荷底 | `status` |
| `info`（預設） | 純灰字、無底 | `status` |

```tsx
{error ? <Message tone="error">{error}</Message> : null}
<Message tone={error ? "error" : "info"}>{error ?? "載入中…"}</Message>
```

### BottomNav — `components/bottom-nav.tsx`

四個固定分頁：今日 / 記錄 / 群組 / 我的。圖示 + 文字，目前分頁是珊瑚色膠囊底。半透明卡片底色 + backdrop blur，內容可以從底下透出來。

`aria-current="page"` 標記目前分頁；顏色不是唯一線索（圖示也放大）。

### 頁面骨架（pattern，未元件化）

```tsx
<main className="flex flex-1 flex-col gap-6 p-6">
  <h1>標題</h1>
  …
</main>
```

只有 9 個使用點且各頁 gap 需求不同，抽成元件的收益還不夠；等到需要統一 header 行為（返回鍵、標題列）再抽。

### 卡片 — `card-surface` utility

`@utility card-surface` = `bg-card rounded-2xl border p-4 shadow-pop`。列表項目、群組卡用它。純 CSS utility，不需要元件。

## Do / Don't

| ✅ Do | ❌ Don't |
|---|---|
| 用語意 token（`bg-primary`、`text-muted-foreground`） | 寫死 hex、oklch、`bg-[#ff6b35]` |
| `<h1>標題</h1>` | `<h1 className="text-2xl font-semibold">` — 會蓋掉全域字級 |
| 錯誤用 `<Message tone="error">` | 自己拼 `text-destructive text-sm` |
| 主色只給主要行動 | 整頁都是橘色 |
| 新增裝飾色時加進 `--mango` 那組 | 為了單一頁面新增語意 token |

## 目前缺口

- 無 dark mode 切換器（token 已備）。
- 無 Badge / Modal / Toast——等實際使用點出現再建。Avatar 已於 slice 6 建立。
- 動效只有按壓與 spinner；照片上傳、訊息進場的動畫留給 Slice 8 UX polish。

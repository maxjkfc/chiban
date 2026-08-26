# App icon 提案

四個方向的 icon 母稿,發布成 design canvas 讓人挑一個:
<https://claude.ai/code/artifact/3f4ad9ea-cb42-4d9c-8521-3f965ce760f2>

- `gen.mjs` — 唯一的來源。四個 icon 的 SVG 跟所有 artboard 都從這裡產生,
  不要直接改 `.dc.html`,改完 `node gen.mjs` 會蓋掉。
- `oklch.mjs` — 把 `apps/web/app/globals.css` 的 oklch token 換算成 hex。
  icon 不能用 CSS variable,但顏色必須跟 app 一致,所以先算出來寫死。
- `canvas.json` — artboard 在畫布上的排法。

重畫並重新發布:

```sh
node gen.mjs
node "<design skill>/seed-canvas.mjs" \
  --template "<design skill>/payload.template.html" \
  --out chiban-app-icon.html --title "吃伴 App Icon" \
  --artboard Main.dc.html --artboard HomeScreen.dc.html \
  --artboard A_Polaroid.dc.html --artboard B_Bowl.dc.html \
  --artboard C_Two.dc.html --artboard D_Carved.dc.html \
  --canvas canvas.json
```

`gen.mjs` 另外會產生 `proof.html` —— 把四個 icon 並排在 256/180/120/62/32px。
那張才是判斷 icon 好壞的依據,母稿好看不算數。兩個只有在 proof 上才看得出來的
問題:A 的紙膠帶原本被 iOS 圓角遮罩切掉,C 原本會讀成一副墨鏡。

選定之後要做的事:轉成 `apps/web/app/icon.svg` 與 apple-touch-icon,換掉
目前還是 Next.js 預設的 `apps/web/app/favicon.ico`。若選 D,「吃」字必須轉成
path —— 靠字型會因為裝置有沒有裝中文襯線體而長得不一樣。

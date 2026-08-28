import { writeFileSync } from 'node:fs';
import { CHI, chiAt } from './glyph.mjs';

const C = {
  coral: '#c13f29', coralInk: '#af2d18', coralDeep: '#8d1d0a', coralLite: '#ea6e52',
  cream: '#f7f3e9', ink: '#2b221a', white: '#ffffff', mango: '#f9a129',
  mint: '#37c695', sky: '#37aae3', berry: '#ac47c2', muted: '#6b5a4d',
  border: '#e5ddd0', yellow: '#fae8a2', paper: '#efe7d8',
};

const CJK_SERIF = `Georgia, &#39;Songti TC&#39;, &#39;Noto Serif TC&#39;, &#39;Source Han Serif TC&#39;, serif`;
const SANS = `-apple-system, &#39;PingFang TC&#39;, &#39;Noto Sans TC&#39;, system-ui, sans-serif`;

/* ---------- the four icon masters, one 1024x1024 viewBox each ---------- */



/* ---------- 吃 as three seal treatments ----------
   The outline comes from Noto Serif TC 900 (see glyph.mjs), so the letterform
   is correct; the design is what is done to it. 朱文 / 白文 is the real
   distinction between the two kinds of Chinese seal: character raised and
   inked, or carved away and left blank. */

const chi = (T, fill) =>
  `<g transform="${chiAt(T)}"><path d="${CHI}" fill="${fill}"/></g>`;

/* 朱文: the character is raised, so it takes the ink and the paper stays bare. */
const D_VERMILION = `
  <defs>
    <pattern id="grainD" width="26" height="26" patternUnits="userSpaceOnUse">
      <circle cx="3" cy="3" r="3" fill="#b89f7c" opacity="0.2"/>
    </pattern>
  </defs>
  <rect width="1024" height="1024" fill="${C.cream}"/>
  <rect width="1024" height="1024" fill="url(#grainD)"/>
  <rect x="144" y="144" width="736" height="736" rx="60" fill="none" stroke="${C.coral}" stroke-width="30"/>
  ${chi(560, C.coral)}`;

/* 白文: the character is cut away, so the block inks around it. */
const D_INTAGLIO = `
  <rect width="1024" height="1024" fill="${C.coral}"/>
  <rect x="130" y="130" width="764" height="764" rx="56" fill="none" stroke="${C.cream}" stroke-width="22"/>
  ${chi(600, C.cream)}`;

const D_CROPPED = `
  <rect width="1024" height="1024" fill="${C.coral}"/>
  ${chi(1180, C.cream)}`;


/* ---------- 黑貓 ----------
   The cat is `--foreground` (#2b221a), not #000: the design system has no pure
   black, and a true black next to warm cream reads as a hole rather than a cat.
   All three put the cat on a light ground — #2b221a on coral is about 3:1,
   which loses the silhouette, and a black cat is nothing but silhouette. */

const GRAIN = `
  <defs>
    <pattern id="grainCat" width="26" height="26" patternUnits="userSpaceOnUse">
      <circle cx="3" cy="3" r="3" fill="#b89f7c" opacity="0.2"/>
    </pattern>
  </defs>
  <rect width="1024" height="1024" fill="${C.cream}"/>
  <rect width="1024" height="1024" fill="url(#grainCat)"/>`;

/* 偷吃的貓: ears and eyes clear the rim, the rest of the cat is behind it. */
const CAT_RAID = `${GRAIN}
  <g fill="${C.ink}">
    <path d="M 344 306 L 398 178 L 458 292 Z"/>
    <path d="M 680 306 L 626 178 L 566 292 Z"/>
    <ellipse cx="512" cy="392" rx="192" ry="172"/>
  </g>
  <g fill="${C.cream}">
    <ellipse cx="440" cy="378" rx="36" ry="27"/>
    <ellipse cx="584" cy="378" rx="36" ry="27"/>
  </g>
  <g fill="${C.ink}">
    <ellipse cx="440" cy="378" rx="10" ry="21"/>
    <ellipse cx="584" cy="378" rx="10" ry="21"/>
  </g>
  <path d="M 490 442 H 534 L 512 468 Z" fill="${C.coral}"/>
  <rect x="196" y="520" width="632" height="64" rx="32" fill="${C.coral}"/>
  <path d="M 240 604 H 784 C 784 764, 664 828, 512 828 C 360 828, 240 764, 240 604 Z" fill="${C.coral}"/>
  <g fill="${C.ink}">
    <rect x="232" y="496" width="98" height="56" rx="28"/>
    <rect x="696" y="496" width="98" height="56" rx="28"/>
  </g>`;

/* 貓耳碗: the bowl IS the cat. Two ears and a tail, nothing else. */
const CAT_BOWL = `${GRAIN}
  <g fill="${C.ink}">
    <path d="M 300 442 L 354 302 L 418 442 Z"/>
    <path d="M 606 442 L 670 302 L 724 442 Z"/>
  </g>
  <path d="M 628 752 C 762 792, 872 754, 888 656 C 896 606, 862 578, 834 598"
        fill="none" stroke="${C.ink}" stroke-width="42" stroke-linecap="round"/>
  <rect x="210" y="430" width="604" height="62" rx="31" fill="${C.coral}"/>
  <path d="M 252 512 H 772 C 772 676, 656 740, 512 740 C 368 740, 252 676, 252 512 Z" fill="${C.coral}"/>`;

/* 睡在對話框裡的貓: direction B's bubble, with the cat curled up asleep in it. */
const CAT_SLEEP = `
  <rect width="1024" height="1024" fill="${C.coral}"/>
  <path d="M 240 168 H 784 a 96 96 0 0 1 96 96 V 664 a 96 96 0 0 1 -96 96 H 404 L 292 892 L 300 760 H 240 a 96 96 0 0 1 -96 -96 V 264 a 96 96 0 0 1 96 -96 Z" fill="${C.cream}"/>
  <g fill="${C.ink}">
    <ellipse cx="530" cy="556" rx="196" ry="128"/>
    <path d="M 300 446 L 326 354 L 382 408 Z"/>
    <path d="M 444 446 L 418 354 L 362 408 Z"/>
    <circle cx="372" cy="500" r="86"/>
  </g>
  <path d="M 716 590 C 742 682, 640 706, 500 694"
        fill="none" stroke="${C.ink}" stroke-width="38" stroke-linecap="round"/>
  <path d="M 336 496 C 350 480, 376 480, 390 496"
        fill="none" stroke="${C.cream}" stroke-width="12" stroke-linecap="round"/>`;


/* ---------- 現行 logo ----------
   Shipped to main in #51 (「湯湯水水」). Reproduced verbatim from
   apps/web/public/logo/chiban-favicon.svg so the comparison is honest — its
   own corner radius is dropped because tile() supplies the mask. */

const SHIPPED = `
  <rect width="1024" height="1024" fill="#C13F29"/>
  <g transform="translate(-96.3 -86.8) scale(19.01)">
    <path fill="#FFFBF4" d="M11 29 A21 21 0 0 0 53 29 Z"/>
    <path transform="translate(21 27) rotate(-8) scale(1.12)" d="M-6.58 -2.39 L-7.5 -12.5 L-2.39 -6.58 A7 7 0 0 1 2.39 -6.58 L7.5 -12.5 L6.58 -2.39 A7 7 0 1 1 -6.58 -2.39 Z" fill="#C13F29" stroke="#C13F29" stroke-width="2.5" stroke-linejoin="round"/>
    <path transform="translate(21 27) rotate(-8) scale(1.12)" d="M-6.58 -2.39 L-7.5 -12.5 L-2.39 -6.58 A7 7 0 0 1 2.39 -6.58 L7.5 -12.5 L6.58 -2.39 A7 7 0 1 1 -6.58 -2.39 Z" fill="#FFFBF4"/>
    <path transform="translate(43.5 27) rotate(7) scale(1.05)" d="M-6.58 -2.39 L-7.5 -12.5 L-2.39 -6.58 A7 7 0 0 1 2.39 -6.58 L7.5 -12.5 L6.58 -2.39 A7 7 0 1 1 -6.58 -2.39 Z" fill="#C13F29" stroke="#C13F29" stroke-width="2.5" stroke-linejoin="round"/>
    <path transform="translate(43.5 27) rotate(7) scale(1.05)" d="M-6.58 -2.39 L-7.5 -12.5 L-2.39 -6.58 A7 7 0 0 1 2.39 -6.58 L7.5 -12.5 L6.58 -2.39 A7 7 0 1 1 -6.58 -2.39 Z" fill="#FFFBF4"/>
  </g>`;

const art = {
  A: `
  <rect width="1024" height="1024" fill="${C.coral}"/>
  <g transform="rotate(-5 512 512)">
    <rect x="228" y="196" width="568" height="640" rx="10" fill="${C.white}"/>
    <rect x="276" y="244" width="472" height="452" rx="6" fill="${C.mango}"/>
    <circle cx="512" cy="470" r="148" fill="${C.cream}"/>
    <circle cx="512" cy="470" r="90" fill="${C.coralLite}"/>
    <circle cx="470" cy="432" r="27" fill="${C.cream}" opacity="0.7"/>
  </g>
  <g transform="rotate(-45 340 290)">
    <rect x="220" y="258" width="240" height="64" rx="3" fill="${C.mint}" opacity="0.92"/>
  </g>`,

  B: `
  <rect width="1024" height="1024" fill="${C.coral}"/>
  <path d="M 240 168 H 784 a 96 96 0 0 1 96 96 V 664 a 96 96 0 0 1 -96 96 H 404 L 292 892 L 300 760 H 240 a 96 96 0 0 1 -96 -96 V 264 a 96 96 0 0 1 96 -96 Z" fill="${C.cream}"/>
  <g stroke="${C.coral}" stroke-width="36" stroke-linecap="round">
    <path d="M 348 398 L 688 266"/>
    <path d="M 366 450 L 706 318"/>
  </g>
  <rect x="300" y="498" width="424" height="54" rx="27" fill="${C.coral}"/>
  <path d="M 338 566 H 686 C 686 668, 608 712, 512 712 C 416 712, 338 668, 338 566 Z" fill="${C.coral}"/>`,

  C: `
  <defs>
    <pattern id="grainC" width="26" height="26" patternUnits="userSpaceOnUse">
      <circle cx="3" cy="3" r="3" fill="#b89f7c" opacity="0.2"/>
    </pattern>
  </defs>
  <rect width="1024" height="1024" fill="${C.cream}"/>
  <rect width="1024" height="1024" fill="url(#grainC)"/>
  <g transform="rotate(9 380 510)">
    <rect x="210" y="380" width="340" height="46" rx="23" fill="${C.coral}"/>
    <path d="M 220 444 H 540 C 540 586, 474 644, 380 644 C 286 644, 220 586, 220 444 Z" fill="${C.coral}"/>
  </g>
  <g transform="rotate(-9 692 540)">
    <rect x="552" y="426" width="280" height="42" rx="21" fill="${C.mango}"/>
    <path d="M 561 486 H 823 C 823 604, 769 652, 692 652 C 615 652, 561 604, 561 486 Z" fill="${C.mango}"/>
  </g>`,

  D: `${D_INTAGLIO}`,
  D1: `${D_VERMILION}`,
  D2: `${D_INTAGLIO}`,
  D3: `${D_CROPPED}`,
  E1: `${CAT_RAID}`,
  E2: `${CAT_BOWL}`,
  E3: `${CAT_SLEEP}`,
  NOW: `${SHIPPED}`,
};

const icon = (k, size) =>
  `<svg viewBox="0 0 1024 1024" width="${size}" height="${size}" style="display: block" role="img" aria-label="方向 ${k}">${art[k]}</svg>`;

/* an iOS-masked tile at any size */
const tile = (k, size) =>
  `<div style="width: ${size}px; height: ${size}px; border-radius: 22.37%; overflow: hidden; flex: none">${icon(k, size)}</div>`;

const DIRS = {
  A: {
    name: '拍立得',
    sub: 'Polaroid & tape',
    idea: '直接沿用 app 的視覺語言:白色相紙、傾斜的角度、一條紙膠帶。看到 icon 就知道打開會是什麼。',
    cost: '小尺寸只剩「一張白色斜方塊」,辨識度靠角度撐,跟其他筆記類 app 容易混。',
  },
  B: {
    name: '一碗話',
    sub: 'Bowl in a bubble',
    idea: '對話框裡放一副碗筷。這支 app 的核心不是吃,是「吃完之後那段對話」,外框直接把它講出來。',
    cost: '兩層資訊(框 + 碗),32px 時碗會糊成一團,只剩對話框可讀。',
  },
  C: {
    name: '一起',
    sub: 'Two bowls',
    idea: '一大一小兩個碗,朝著彼此傾斜 —「伴」字的意思。唯一的淺色底,在一排深色 app 之間反而跳出來。',
    cost: '淺色底在淺色桌布上會沉下去,而且縮到 32px 時兩個碗容易被看成一副墨鏡。',
  },
  D: {
    name: '吃字印',
    sub: 'Carved seal',
    idea: '把「吃」刻成一枚印章。滿螢幕的英文 logo 跟漸層之間,一個中文字最不像別人。',
    cost: '筆畫最多的一個,32px 一定糊。中文字對非中文使用者是一道牆。',
  },
};

const head = (extra = '') => `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <script src="./support.js"></script>
</head>
<body>
<x-dc>
<helmet>
  <style>
    body { margin: 0; font-family: ${SANS}; }
    a { color: ${C.coralInk}; } a:hover { color: ${C.coralDeep}; }
    ${extra}
  </style>
</helmet>`;

const foot = `</x-dc>
</body>
</html>
`;

/* ---------- Main: the decision sheet ---------- */

const col = (k) => {
  const d = DIRS[k];
  return `
      <div style="display: flex; flex-direction: column; gap: 20px">
        ${tile(k, 240)}
        <div style="display: flex; flex-direction: column; gap: 6px">
          <div style="display: flex; align-items: baseline; gap: 10px">
            <span style="font-family: ${CJK_SERIF}; font-size: 15px; color: ${C.coralInk}; letter-spacing: 0.14em">${k}</span>
            <span style="font-family: ${CJK_SERIF}; font-size: 27px; color: ${C.ink}">${d.name}</span>
          </div>
          <div style="font-size: 13px; color: ${C.muted}; letter-spacing: 0.06em">${d.sub}</div>
        </div>
        <p style="margin: 0; font-size: 14px; line-height: 1.75; color: ${C.ink}; text-wrap: pretty">${d.idea}</p>
        <p style="margin: 0; font-size: 13px; line-height: 1.7; color: ${C.muted}; text-wrap: pretty">
          <span style="color: ${C.coralInk}">取捨 &#183;</span> ${d.cost}
        </p>
        <div style="display: flex; align-items: flex-end; gap: 18px; margin-top: 4px; padding-top: 20px; border-top: 1px solid ${C.border}">
          ${tile(k, 64)}
          ${tile(k, 32)}
          <span style="font-size: 11px; color: ${C.muted}; letter-spacing: 0.08em; padding-bottom: 2px">64 &#183; 32</span>
        </div>
      </div>`;
};

writeFileSync('Main.dc.html', head() + `
<div style="width: 1360px; min-height: 940px; box-sizing: border-box; padding: 64px; background: ${C.cream};
            background-image: radial-gradient(circle at 1px 1px, rgba(184,159,124,0.22) 1px, transparent 0);
            background-size: 7px 7px">
  <div style="display: flex; flex-direction: column; gap: 8px; margin-bottom: 48px">
    <h1 style="margin: 0; font-family: ${CJK_SERIF}; font-size: 44px; font-weight: 400; color: ${C.ink}">吃伴 &#183; App Icon 四個方向</h1>
    <p style="margin: 0; font-size: 15px; color: ${C.muted}; line-height: 1.7">
      顏色全部取自 v0.2 的 token,沒有新增色。挑一個,我再把它做成 <span style="font-family: ui-monospace, monospace; font-size: 13px; color: ${C.coralInk}">app/icon.svg</span> 跟 apple-touch-icon 接進去。
    </p>
  </div>
  <div style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 48px">
    ${['A','B','C','D'].map(col).join('')}
  </div>
</div>` + foot);

/* ---------- per-direction detail sheets, 1024 master at 1:1 ---------- */

const rung = (k, size, label) => `
        <div style="display: flex; flex-direction: column; align-items: center; gap: 12px">
          ${tile(k, size)}
          <span style="font-size: 11px; color: ${C.muted}; letter-spacing: 0.1em">${label}</span>
        </div>`;

for (const k of ['A', 'B', 'C', 'D']) {
  const d = DIRS[k];
  writeFileSync(`${k}_${d.sub.split(' ')[0]}.dc.html`, head() + `
<div style="width: 1240px; min-height: 1620px; box-sizing: border-box; padding: 72px; background: ${C.cream};
            background-image: radial-gradient(circle at 1px 1px, rgba(184,159,124,0.22) 1px, transparent 0);
            background-size: 7px 7px; display: flex; flex-direction: column; gap: 56px">

  <div style="display: flex; flex-direction: column; gap: 10px">
    <div style="display: flex; align-items: baseline; gap: 14px">
      <span style="font-family: ${CJK_SERIF}; font-size: 18px; color: ${C.coralInk}; letter-spacing: 0.16em">方向 ${k}</span>
      <h1 style="margin: 0; font-family: ${CJK_SERIF}; font-size: 40px; font-weight: 400; color: ${C.ink}">${d.name}</h1>
      <span style="font-size: 14px; color: ${C.muted}; letter-spacing: 0.08em">${d.sub}</span>
    </div>
    <p style="margin: 0; max-width: 720px; font-size: 15px; line-height: 1.8; color: ${C.ink}; text-wrap: pretty">${d.idea}</p>
    <p style="margin: 0; max-width: 720px; font-size: 14px; line-height: 1.75; color: ${C.muted}; text-wrap: pretty">
      <span style="color: ${C.coralInk}">取捨 &#183;</span> ${d.cost}
    </p>
  </div>

  <div style="display: flex; flex-direction: column; gap: 20px">
    <div style="font-size: 12px; color: ${C.muted}; letter-spacing: 0.12em">1024 &#215; 1024 &#183; 母稿(實際大小)</div>
    <div style="display: flex; gap: 40px; align-items: flex-start">
      <div style="width: 1024px; height: 1024px; border-radius: 22.37%; overflow: hidden; flex: none; box-shadow: 0 24px 48px -32px rgba(64,40,24,0.55)">
        ${icon(k, 1024)}
      </div>
    </div>
  </div>

  <div style="display: flex; flex-direction: column; gap: 24px; padding-top: 44px; border-top: 1px solid ${C.border}">
    <div style="font-size: 12px; color: ${C.muted}; letter-spacing: 0.12em">實際使用尺寸(1:1,沒有放大)</div>
    <div style="display: flex; align-items: flex-end; gap: 56px">
      ${rung(k, 180, '180 &#183; iPhone 主畫面 @3x')}
      ${rung(k, 64, '64 &#183; 設定 / 分享')}
      ${rung(k, 32, '32 &#183; favicon')}
      <div style="display: flex; flex-direction: column; gap: 10px; padding-bottom: 4px; max-width: 300px">
        <span style="font-size: 12px; color: ${C.coralInk}; letter-spacing: 0.1em">看這一排,不要看上面那張</span>
        <span style="font-size: 13px; line-height: 1.7; color: ${C.muted}; text-wrap: pretty">icon 的成敗只在這三個尺寸。母稿好看不算數。</span>
      </div>
    </div>
  </div>

  <div style="display: flex; flex-direction: column; gap: 24px; padding-top: 44px; border-top: 1px solid ${C.border}">
    <div style="font-size: 12px; color: ${C.muted}; letter-spacing: 0.12em">遮罩</div>
    <div style="display: flex; align-items: center; gap: 48px">
      <div style="display: flex; flex-direction: column; align-items: center; gap: 12px">
        ${tile(k, 140)}
        <span style="font-size: 11px; color: ${C.muted}; letter-spacing: 0.1em">iOS squircle</span>
      </div>
      <div style="display: flex; flex-direction: column; align-items: center; gap: 12px">
        <div style="width: 140px; height: 140px; border-radius: 50%; overflow: hidden; flex: none">${icon(k, 140)}</div>
        <span style="font-size: 11px; color: ${C.muted}; letter-spacing: 0.1em">Android 圓形</span>
      </div>
      <div style="display: flex; flex-direction: column; align-items: center; gap: 12px">
        <div style="width: 140px; height: 140px; overflow: hidden; flex: none">${icon(k, 140)}</div>
        <span style="font-size: 11px; color: ${C.muted}; letter-spacing: 0.1em">無遮罩(方形)</span>
      </div>
    </div>
  </div>
</div>` + foot);
}


/* ---------- D 的三種刻法 ---------- */

const HANDS = [
  ['D1', '朱文', 'chi(560) 蓋在紙上', '字是凸起的,所以字吃墨、紙留白。最斯文的一個,也是唯一的淺色版。'],
  ['D2', '白文', 'chi(600) 刻進石頭', '字被刻掉,墨留在字的四周。筆畫對比最強,縮到最小還撐得住。'],
  ['D3', '破框', 'chi(1180) 讓邊界切它', '字大到被 icon 邊界裁掉。不再是一個字,是一個記號。'],
];

const hand = ([k, name, how, why]) => `
      <div style="display: flex; flex-direction: column; gap: 18px">
        ${tile(k, 256)}
        <div style="display: flex; align-items: baseline; gap: 10px">
          <span style="font-family: ${CJK_SERIF}; font-size: 26px; color: ${C.ink}">${name}</span>
          <span style="font-family: ui-monospace, monospace; font-size: 12px; color: ${C.muted}">${how}</span>
        </div>
        <p style="margin: 0; font-size: 14px; line-height: 1.75; color: ${C.ink}; text-wrap: pretty">${why}</p>
        <div style="display: flex; align-items: flex-end; gap: 18px; padding-top: 18px; border-top: 1px solid ${C.border}">
          ${tile(k, 62)}
          ${tile(k, 32)}
          <span style="font-size: 11px; color: ${C.muted}; letter-spacing: 0.08em; padding-bottom: 2px">62 &#183; 32</span>
        </div>
      </div>`;

writeFileSync('D_Letterforms.dc.html', head() + `
<div style="width: 1240px; min-height: 980px; box-sizing: border-box; padding: 72px; background: ${C.cream};
            background-image: radial-gradient(circle at 1px 1px, rgba(184,159,124,0.22) 1px, transparent 0);
            background-size: 7px 7px; display: flex; flex-direction: column; gap: 48px">
  <div style="display: flex; flex-direction: column; gap: 12px">
    <h1 style="margin: 0; font-family: ${CJK_SERIF}; font-size: 38px; font-weight: 400; color: ${C.ink}">D &#183; 三種刻法</h1>
    <p style="margin: 0; max-width: 860px; font-size: 15px; line-height: 1.8; color: ${C.ink}; text-wrap: pretty">
      第一版的「吃」是用系統字型排出來的,所以它長什麼樣子完全不是設計的結果 —— 那台機器剛好裝什麼字體就是什麼。
      現在這個字是 Noto Serif TC 900 的外框(SIL OFL,可自由衍生),存成 path。明體本身的粗細對比跟三角襯線就是特色,而且在沒裝中文襯線體的裝置上長得一模一樣。
    </p>
    <p style="margin: 0; max-width: 860px; font-size: 14px; line-height: 1.75; color: ${C.muted}; text-wrap: pretty">
      朱文與白文是真的兩種印:字凸起來吃墨,或字被刻掉、墨留四周。
    </p>
  </div>
  <div style="display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 44px">
    ${HANDS.map(hand).join('')}
  </div>
</div>` + foot);


/* ---------- E · 黑貓 ---------- */

const CATS = [
  ['E1', '偷吃的貓', '一隻黑貓從碗緣探出頭,兩隻前爪搭在碗上。這是四個字說不出來的東西:吃飯這件事本身是開心的。'],
  ['E2', '貓耳碗', '碗就是貓 —— 兩隻耳朵加一條尾巴,沒有別的。三個裡面縮到最小還撐得住的一個。'],
  ['E3', '睡在對話框裡', 'B 的對話框,裡面睡了一隻貓。吃飽、聊完、睡著 —— 這支 app 的一天。'],
];

const catCol = ([k, name, why]) => `
      <div style="display: flex; flex-direction: column; gap: 18px">
        ${tile(k, 256)}
        <div style="display: flex; align-items: baseline; gap: 10px">
          <span style="font-family: ${CJK_SERIF}; font-size: 15px; color: ${C.coralInk}; letter-spacing: 0.14em">${k}</span>
          <span style="font-family: ${CJK_SERIF}; font-size: 26px; color: ${C.ink}">${name}</span>
        </div>
        <p style="margin: 0; font-size: 14px; line-height: 1.75; color: ${C.ink}; text-wrap: pretty">${why}</p>
        <div style="display: flex; align-items: flex-end; gap: 18px; padding-top: 18px; border-top: 1px solid ${C.border}">
          ${tile(k, 62)}
          ${tile(k, 32)}
          <span style="font-size: 11px; color: ${C.muted}; letter-spacing: 0.08em; padding-bottom: 2px">62 &#183; 32</span>
        </div>
      </div>`;

writeFileSync('E_BlackCat.dc.html', head() + `
<div style="width: 1240px; min-height: 1020px; box-sizing: border-box; padding: 72px; background: ${C.cream};
            background-image: radial-gradient(circle at 1px 1px, rgba(184,159,124,0.22) 1px, transparent 0);
            background-size: 7px 7px; display: flex; flex-direction: column; gap: 48px">
  <div style="display: flex; flex-direction: column; gap: 12px">
    <h1 style="margin: 0; font-family: ${CJK_SERIF}; font-size: 38px; font-weight: 400; color: ${C.ink}">E &#183; 黑貓</h1>
    <p style="margin: 0; max-width: 880px; font-size: 15px; line-height: 1.8; color: ${C.ink}; text-wrap: pretty">
      貓是 <span style="font-family: ui-monospace, monospace; font-size: 13px">#2b221a</span>,不是純黑 —— 設計系統裡沒有純黑,而純黑貼著奶油紙會讀成一個洞,不是一隻貓。
    </p>
    <p style="margin: 0; max-width: 880px; font-size: 14px; line-height: 1.75; color: ${C.muted}; text-wrap: pretty">
      三個都把貓放在淺底上。黑貓在珊瑚紅上只有 3:1 左右的對比,剪影會糊掉 —— 而黑貓除了剪影什麼都不是。
    </p>
  </div>
  <div style="display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 44px">
    ${CATS.map(catCol).join('')}
  </div>
</div>` + foot);

/* ---------- home screen comparison ---------- */

const neighbour = (bg, mark, label) => `
      <div style="display: flex; flex-direction: column; align-items: center; gap: 7px; width: 62px">
        <div style="width: 62px; height: 62px; border-radius: 22.37%; overflow: hidden; background: ${bg}; flex: none;
                    box-shadow: 0 2px 6px -2px rgba(40,26,14,0.4)">
          <svg viewBox="0 0 62 62" width="62" height="62" style="display: block">${mark}</svg>
        </div>
        <span style="font-size: 10px; color: #3a2f26; letter-spacing: 0.02em; white-space: nowrap">${label}</span>
      </div>`;

const candidate = (k, label) => `
      <div style="display: flex; flex-direction: column; align-items: center; gap: 7px; width: 62px">
        <div style="box-shadow: 0 2px 6px -2px rgba(40,26,14,0.4); border-radius: 22.37%">${tile(k, 62)}</div>
        <span style="font-size: 10px; color: #3a2f26; letter-spacing: 0.02em; white-space: nowrap">${label}</span>
      </div>`;

const NEIGHBOURS = [
  ['#4a5c6a', `<g fill="none" stroke="#e6edf2" stroke-width="3.4" stroke-linecap="round"><rect x="17" y="20" width="28" height="22" rx="4"/><path d="M 24 42 v 5"/></g>`, '相機'],
  ['#e8e4dc', `<g fill="none" stroke="#8a7f72" stroke-width="3.4" stroke-linecap="round"><path d="M 19 22 h 24"/><path d="M 19 31 h 24"/><path d="M 19 40 h 15"/></g>`, '備忘錄'],
  ['#2f7d4f', `<g fill="none" stroke="#eaf5ee" stroke-width="3.4" stroke-linecap="round" stroke-linejoin="round"><path d="M 31 17 L 45 45 L 31 38 L 17 45 Z"/></g>`, '地圖'],
  ['#3b6fd4', `<g fill="none" stroke="#eef3fd" stroke-width="3.4" stroke-linecap="round" stroke-linejoin="round"><path d="M 17 24 a 5 5 0 0 1 5 -5 h 18 a 5 5 0 0 1 5 5 v 12 a 5 5 0 0 1 -5 5 H 27 l -8 7 v -7 h -2 Z"/></g>`, '訊息'],
  ['#d9d2c6', `<g fill="none" stroke="#6f6558" stroke-width="3.4" stroke-linecap="round"><circle cx="31" cy="31" r="13"/><path d="M 31 23 v 8 l 5 4"/></g>`, '時鐘'],
  ['#1f1c1a', `<g fill="none" stroke="#f0ece6" stroke-width="3.4" stroke-linecap="round"><path d="M 22 41 V 26"/><path d="M 31 41 V 19"/><path d="M 40 41 V 32"/></g>`, '健康'],
  ['#c9a227', `<g fill="none" stroke="#fdf7e4" stroke-width="3.4" stroke-linecap="round" stroke-linejoin="round"><path d="M 19 40 L 27 28 L 34 36 L 39 30 L 44 40 Z"/><circle cx="24" cy="23" r="3.2"/></g>`, '相簿'],
  ['#7a5b8f', `<g fill="none" stroke="#f3ecf7" stroke-width="3.4" stroke-linecap="round"><circle cx="31" cy="31" r="12"/><circle cx="31" cy="31" r="3.4"/></g>`, '音樂'],
];

writeFileSync('HomeScreen.dc.html', head() + `
<div style="width: 390px; height: 844px; box-sizing: border-box; background: ${C.paper};
            background-image: radial-gradient(circle at 1px 1px, rgba(150,124,92,0.2) 1px, transparent 0);
            background-size: 6px 6px; display: flex; flex-direction: column; gap: 26px;
            padding: 108px 26px 0">

  <div style="display: grid; grid-template-columns: repeat(4, 62px); justify-content: space-between; gap: 22px 0">
    ${candidate('A', '吃伴 A')}
    ${candidate('B', '吃伴 B')}
    ${candidate('C', '吃伴 C')}
    ${candidate('D', '吃伴 D')}
  </div>

  <div style="display: grid; grid-template-columns: repeat(4, 62px); justify-content: space-between; gap: 22px 0">
    ${candidate('E1', '吃伴 E1')}
    ${candidate('E2', '吃伴 E2')}
    ${candidate('E3', '吃伴 E3')}
    ${candidate('NOW', '現行')}
  </div>

  <div style="display: grid; grid-template-columns: repeat(4, 62px); justify-content: space-between; gap: 22px 0">
    ${NEIGHBOURS.slice(0, 4).map(n => neighbour(...n)).join('')}
    ${NEIGHBOURS.slice(4, 8).map(n => neighbour(...n)).join('')}
  </div>
</div>` + foot);

console.log('wrote Main, HomeScreen, A/B/C/D');

/* proof sheet — for my own eyes only, not part of the canvas */
const proofRow = (size) => `<div style="display:flex;gap:28px;align-items:flex-end;margin:26px 0">
  ${['E1','E2','E3','B','D2'].map(k => tile(k, size)).join('')}
  <span style="font:12px ${SANS};color:#666">${size}px</span></div>`;
writeFileSync('proof.html', `<!doctype html><meta charset="utf-8">
<body style="margin:0;padding:36px;background:#efe7d8;font-family:${SANS}">
${[256,180,120,62,32].map(proofRow).join('')}
</body>`);

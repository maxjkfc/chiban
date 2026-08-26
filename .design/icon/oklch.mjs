// oklch -> sRGB hex (Björn Ottosson's Oklab, sRGB transfer fn)
function oklchToHex(L, C, hDeg) {
  const h = hDeg * Math.PI / 180;
  const a = C * Math.cos(h), b = C * Math.sin(h);
  const l_ = L + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = L - 0.0894841775 * a - 1.2914855480 * b;
  const l = l_ ** 3, m = m_ ** 3, s = s_ ** 3;
  let r =  4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s;
  let g = -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s;
  let bb = -0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s;
  const enc = v => { v = v <= 0.0031308 ? 12.92*v : 1.055*Math.pow(v, 1/2.4) - 0.055;
                     return Math.round(Math.min(1, Math.max(0, v)) * 255); };
  return '#' + [enc(r), enc(g), enc(bb)].map(x => x.toString(16).padStart(2,'0')).join('');
}
const T = {
  primary:[0.55,0.17,32], 'primary-ink':[0.5,0.17,32], background:[0.965,0.014,88],
  foreground:[0.26,0.02,60], card:[1,0,0], secondary:[0.93,0.09,95],
  'secondary-foreground':[0.36,0.07,70], muted:[0.94,0.022,82], 'muted-foreground':[0.48,0.03,60],
  border:[0.9,0.02,80], mango:[0.78,0.16,68], mint:[0.74,0.14,165], sky:[0.7,0.13,235],
  berry:[0.58,0.2,320], 'tape-1':[0.62,0.14,30], 'tape-2':[0.62,0.14,95],
  'tape-3':[0.62,0.14,200], 'tape-4':[0.62,0.14,330],
  'coral-deep':[0.42,0.15,32], 'coral-light':[0.68,0.16,34],
};
for (const [k,v] of Object.entries(T)) console.log(k.padEnd(22), oklchToHex(...v));

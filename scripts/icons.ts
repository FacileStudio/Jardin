// Regenerates every brand asset in apps/client/static from one source of truth.
//
// Run it rather than editing the binaries: favicon.svg, favicon.ico, the three
// PNG launcher icons and the og-image all carry the same mark, and hand-editing
// one is how the set drifted before — the SVG carried a four-petal glyph while
// every PNG carried an egg, and the og-image still advertised the old name and a
// domain that answers 404.
//
// It needs one tool that is deliberately not a project dependency, since this
// runs when the brand changes and never at build time:
//
//   bun add @resvg/resvg-js && bun run scripts/icons.ts
//
// favicon.ico is written by hand because no encoder here emits one: three BMP
// entries at 16/32/48 and 32bpp, which is the shape the file it replaces had.

import { Resvg } from '@resvg/resvg-js'
import { writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const OUT = join(dirname(fileURLToPath(import.meta.url)), '..', 'apps', 'client', 'static')
const INK = '#fafafa'
const BG = '#0a0a0a'

// solar:structure-bold-duotone — four solid nodes joined by a half-opacity
// lattice. The nodes are the machines and agents; the lattice is the shared
// store between them, which is the whole product in one glyph. It also keeps
// the 2x2 corner rhythm of the mark it replaces, so this reads as a rename.
const NODES = [
  'M8 5C8 6.65685 6.65685 8 5 8C3.34315 8 2 6.65685 2 5C2 3.34315 3.34315 2 5 2C6.65685 2 8 3.34315 8 5Z',
  'M22 5C22 6.65685 20.6569 8 19 8C17.3431 8 16 6.65685 16 5C16 3.34315 17.3431 2 19 2C20.6569 2 22 3.34315 22 5Z',
  'M8 19C8 20.6569 6.65685 22 5 22C3.34315 22 2 20.6569 2 19C2 17.3431 3.34315 16 5 16C6.65685 16 8 17.3431 8 19Z',
  'M22 19C22 20.6569 20.6569 22 19 22C17.3431 22 16 20.6569 16 19C16 17.3431 17.3431 16 19 16C20.6569 16 22 17.3431 22 19Z',
]
const LATTICE = [
  'M16.093 4.25572C16.0626 4.25194 16.0315 4.25 16 4.25H8C7.96849 4.25 7.93744 4.25194 7.90695 4.25572C7.9677 4.49371 8 4.74308 8 5C8 5.25692 7.9677 5.50629 7.90695 5.74428C7.93744 5.74806 7.96849 5.75 8 5.75H16C16.0315 5.75 16.0626 5.74805 16.093 5.74428C16.0323 5.50629 16 5.25692 16 5C16 4.74308 16.0323 4.49371 16.093 4.25572Z',
  'M19 8C19.2569 8 19.5063 7.96771 19.7443 7.90695C19.7481 7.93744 19.75 7.96849 19.75 8V16C19.75 16.0315 19.7481 16.0626 19.7443 16.0931C19.5063 16.0323 19.2569 16 19 16C18.7431 16 18.4937 16.0323 18.2557 16.0931C18.2519 16.0626 18.25 16.0315 18.25 16V8C18.25 7.96849 18.2519 7.93744 18.2557 7.90695C18.4937 7.96771 18.7431 8 19 8Z',
  'M16.0931 18.2557C16.0626 18.2519 16.0315 18.25 16 18.25H8C7.96849 18.25 7.93744 18.2519 7.90695 18.2557C7.9677 18.4937 8 18.7431 8 19C8 19.2569 7.9677 19.5063 7.90695 19.7443C7.93744 19.7481 7.96849 19.75 8 19.75H16C16.0315 19.75 16.0626 19.7481 16.0931 19.7443C16.0323 19.5063 16 19.2569 16 19C16 18.7431 16.0323 18.4937 16.0931 18.2557Z',
  'M5 8C4.74308 8 4.49371 7.9677 4.25572 7.90695C4.25194 7.93744 4.25 7.96849 4.25 8V16C4.25 16.0315 4.25194 16.0626 4.25572 16.093C4.49371 16.0323 4.74308 16 5 16C5.25692 16 5.50629 16.0323 5.74428 16.0931C5.74806 16.0626 5.75 16.0315 5.75 16L5.75 8C5.75 7.96849 5.74806 7.93744 5.74428 7.90695C5.50629 7.9677 5.25692 8 5 8Z',
]

const glyph = (fill: string) =>
  `<g fill="${fill}">${NODES.map((d) => `<path d="${d}"/>`).join('')}` +
  `<g opacity=".5">${LATTICE.map((d) => `<path d="${d}"/>`).join('')}</g></g>`

// The tab icon stays theme-aware the way the one it replaces was: one asset
// that inverts with the reader's scheme rather than two that can drift.
const faviconSvg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
\t<style>
\t\t:root { color: ${BG} }
\t\t@media (prefers-color-scheme: dark) { :root { color: ${INK} } }
\t</style>
\t${glyph('currentColor')}
</svg>
`
writeFileSync(`${OUT}/favicon.svg`, faviconSvg)

// A launcher icon cannot be theme-aware, so it commits to the dark squircle
// the suite already uses, matching theme_color in the manifest.
const markSvg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <rect width="100" height="100" rx="22" fill="${BG}"/>
  <g transform="translate(26 26) scale(2)">${glyph(INK)}</g>
</svg>`

const render = (svg: string, width: number) =>
  new Resvg(svg, { fitTo: { mode: 'width', value: width }, font: { loadSystemFonts: true } }).render()

for (const [name, size] of [['apple-touch-icon.png', 180], ['icon-192.png', 192], ['icon-512.png', 512]] as const) {
  writeFileSync(`${OUT}/${name}`, render(markSvg, size).asPng())
  console.log(`  wrote ${name} (${size}x${size})`)
}

// --- favicon.ico: 16/32/48, 32bpp BMP entries, matching what it replaces ---
const sizes = [16, 32, 48]
const images = sizes.map((s) => {
  const r = render(markSvg, s)
  const px = r.pixels as unknown as Buffer // RGBA, top-down
  const rowXor = s * 4
  const andRow = Math.ceil(s / 32) * 4
  const header = Buffer.alloc(40)
  header.writeUInt32LE(40, 0); header.writeInt32LE(s, 4); header.writeInt32LE(s * 2, 8)
  header.writeUInt16LE(1, 12); header.writeUInt16LE(32, 14); header.writeUInt32LE(0, 16)
  header.writeUInt32LE(rowXor * s + andRow * s, 20)
  const xor = Buffer.alloc(rowXor * s)
  for (let y = 0; y < s; y++) {
    const src = (s - 1 - y) * rowXor // BMP is bottom-up
    for (let x = 0; x < s; x++) {
      const i = src + x * 4, o = y * rowXor + x * 4
      xor[o] = px[i + 2]; xor[o + 1] = px[i + 1]; xor[o + 2] = px[i]; xor[o + 3] = px[i + 3]
    }
  }
  return { size: s, data: Buffer.concat([header, xor, Buffer.alloc(andRow * s)]) }
})
const dir = Buffer.alloc(6 + 16 * images.length)
dir.writeUInt16LE(0, 0); dir.writeUInt16LE(1, 2); dir.writeUInt16LE(images.length, 4)
let offset = dir.length
images.forEach((img, i) => {
  const e = 6 + i * 16
  dir.writeUInt8(img.size, e); dir.writeUInt8(img.size, e + 1); dir.writeUInt8(0, e + 2); dir.writeUInt8(0, e + 3)
  dir.writeUInt16LE(1, e + 4); dir.writeUInt16LE(32, e + 6)
  dir.writeUInt32LE(img.data.length, e + 8); dir.writeUInt32LE(offset, e + 12)
  offset += img.data.length
})
writeFileSync(`${OUT}/favicon.ico`, Buffer.concat([dir, ...images.map((i) => i.data)]))
console.log(`  wrote favicon.ico (${sizes.join('/')})`)

// --- og-image: the card that renders when the site is shared ---
const MUTED = '#a1a1aa', FAINT = '#71717a', RULE = '#27272a'
const TAGLINE = [
  'One brain for all your AI coding agents, synced',
  'across every machine. Memory, rules and skills in',
  'a single canonical store.',
]
const ogSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
  <rect width="1200" height="630" fill="${BG}"/>
  <g transform="translate(96 92) scale(4.4)">${glyph(INK)}</g>
  <text x="96" y="312" font-family="Liberation Sans" font-size="86" font-weight="bold" fill="${INK}">Mycelium</text>
  ${TAGLINE.map((line, i) => `<text x="96" y="${386 + i * 50}" font-family="Liberation Sans" font-size="31" fill="${MUTED}">${line}</text>`).join('\n  ')}
  <line x1="96" y1="516" x2="1104" y2="516" stroke="${RULE}" stroke-width="2"/>
  <text x="96" y="566" font-family="Liberation Sans" font-size="23" fill="${FAINT}">mycelium.facile.studio</text>
  <text x="1104" y="566" text-anchor="end" font-family="Liberation Sans" font-size="23" fill="${FAINT}">Facile Studio</text>
</svg>`
const og = new Resvg(ogSvg, { font: { loadSystemFonts: true, defaultFontFamily: 'Liberation Sans' } }).render()
writeFileSync(`${OUT}/og-image.png`, og.asPng())
console.log('  wrote og-image.png (1200x630)')

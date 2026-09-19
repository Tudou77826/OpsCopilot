// 占位素材生成脚本：用坐标、图元和调色板程序拼出 ops-pixel-habitat 的 82 张 PNG。
//
// 正式美术靠绘制产出（手绘或绘图 AI），程序只做一轮归一化，见
// docs/garden-presentation-architecture.md 第 4.2 节。本脚本产出的是占位素材，
// 用于在绘制素材就位前验证运行时、门禁和体积预算，不得当作正式美术管线使用。
import { deflateSync } from 'node:zlib'
import { mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { dirname, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(here, '..', '..')
const packDir = resolve(repoRoot, 'frontend-shell', 'src', 'ui', 'garden', 'packs', 'pixelHabitat')
const assetDir = resolve(packDir, 'assets')
mkdirSync(assetDir, { recursive: true })

const palettes = {
  mint: { outline: '#173b32', stem: '#2d7154', leaf: '#4f9a64', light: '#87c96f', bloom: '#f4cf73' },
  fern: { outline: '#183b32', stem: '#376b46', leaf: '#5c9b58', light: '#8ac46c', bloom: '#d8dc8d' },
  orchid: { outline: '#2f2942', stem: '#3c704d', leaf: '#61915d', light: '#91bd76', bloom: '#c487cf' },
  oak: { outline: '#293629', stem: '#6e4c32', leaf: '#4c7d45', light: '#78a956', bloom: '#d6a84f' },
  shiny: { outline: '#392f62', stem: '#6c58a8', leaf: '#55b6a3', light: '#8ce3ca', bloom: '#ffe68c' },
  fox: { outline: '#352532', body: '#c85f38', light: '#ef9b58', cream: '#f8dda1', dark: '#673b36' },
  foxShiny: { outline: '#2e3158', body: '#6f79c8', light: '#a9b7ef', cream: '#fff0af', dark: '#494d80' },
  bird: { outline: '#24304e', body: '#4f87a9', light: '#83c7c8', cream: '#f7df9b', dark: '#35586e' },
  birdShiny: { outline: '#4a3158', body: '#a45ca9', light: '#e09bd2', cream: '#fff0a8', dark: '#69406f' },
}

function rgba(hex, alpha = 255) {
  const value = Number.parseInt(hex.slice(1), 16)
  return [(value >> 16) & 255, (value >> 8) & 255, value & 255, alpha]
}

function canvas(width, height, fill = [0, 0, 0, 0]) {
  const pixels = new Uint8Array(width * height * 4)
  for (let i = 0; i < width * height; i++) pixels.set(fill, i * 4)
  return { width, height, pixels }
}

function setPixel(image, x, y, color) {
  x = Math.round(x); y = Math.round(y)
  if (x < 0 || y < 0 || x >= image.width || y >= image.height) return
  image.pixels.set(color, (y * image.width + x) * 4)
}

function rect(image, x, y, width, height, color) {
  for (let py = Math.floor(y); py < Math.ceil(y + height); py++) {
    for (let px = Math.floor(x); px < Math.ceil(x + width); px++) setPixel(image, px, py, color)
  }
}

function ellipse(image, cx, cy, rx, ry, color) {
  const minX = Math.floor(cx - rx), maxX = Math.ceil(cx + rx)
  const minY = Math.floor(cy - ry), maxY = Math.ceil(cy + ry)
  for (let y = minY; y <= maxY; y++) {
    for (let x = minX; x <= maxX; x++) {
      if (((x - cx) / rx) ** 2 + ((y - cy) / ry) ** 2 <= 1) setPixel(image, x, y, color)
    }
  }
}

function line(image, x0, y0, x1, y1, color, width = 1) {
  let dx = Math.abs(x1 - x0), sx = x0 < x1 ? 1 : -1
  let dy = -Math.abs(y1 - y0), sy = y0 < y1 ? 1 : -1
  let error = dx + dy
  while (true) {
    rect(image, x0 - Math.floor(width / 2), y0 - Math.floor(width / 2), width, width, color)
    if (x0 === x1 && y0 === y1) break
    const twice = 2 * error
    if (twice >= dy) { error += dy; x0 += sx }
    if (twice <= dx) { error += dx; y0 += sy }
  }
}

function polygon(image, points, color) {
  const minY = Math.floor(Math.min(...points.map(point => point[1])))
  const maxY = Math.ceil(Math.max(...points.map(point => point[1])))
  for (let y = minY; y <= maxY; y++) {
    const intersections = []
    for (let i = 0; i < points.length; i++) {
      const a = points[i], b = points[(i + 1) % points.length]
      if ((a[1] <= y && b[1] > y) || (b[1] <= y && a[1] > y)) intersections.push(a[0] + ((y - a[1]) * (b[0] - a[0])) / (b[1] - a[1]))
    }
    intersections.sort((a, b) => a - b)
    for (let i = 0; i + 1 < intersections.length; i += 2) rect(image, Math.ceil(intersections[i]), y, Math.floor(intersections[i + 1]) - Math.ceil(intersections[i]) + 1, 1, color)
  }
}

function sparkle(image, x, y, color) {
  rect(image, x, y - 2, 1, 5, color)
  rect(image, x - 2, y, 5, 1, color)
}

function scaleNearest(source, scale) {
  const result = canvas(source.width * scale, source.height * scale)
  for (let y = 0; y < source.height; y++) for (let x = 0; x < source.width; x++) {
    const offset = (y * source.width + x) * 4
    rect(result, x * scale, y * scale, scale, scale, source.pixels.subarray(offset, offset + 4))
  }
  return result
}

function concatFrames(frames, scale = 2) {
  const result = canvas(frames.length * frames[0].width * scale, frames[0].height * scale)
  frames.forEach((frame, index) => {
    const scaled = scaleNearest(frame, scale)
    for (let y = 0; y < scaled.height; y++) for (let x = 0; x < scaled.width; x++) {
      const offset = (y * scaled.width + x) * 4
      setPixel(result, index * scaled.width + x, y, scaled.pixels.subarray(offset, offset + 4))
    }
  })
  return result
}

function crc32(buffer) {
  let crc = 0xffffffff
  for (const byte of buffer) {
    crc ^= byte
    for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ (0xedb88320 & -(crc & 1))
  }
  return (crc ^ 0xffffffff) >>> 0
}

function chunk(type, data) {
  const typeBuffer = Buffer.from(type)
  const length = Buffer.alloc(4); length.writeUInt32BE(data.length)
  const checksum = Buffer.alloc(4); checksum.writeUInt32BE(crc32(Buffer.concat([typeBuffer, data])))
  return Buffer.concat([length, typeBuffer, data, checksum])
}

function encodePng(image) {
  const raw = Buffer.alloc((image.width * 4 + 1) * image.height)
  for (let y = 0; y < image.height; y++) {
    const target = y * (image.width * 4 + 1)
    raw[target] = 0
    Buffer.from(image.pixels.buffer, image.pixels.byteOffset + y * image.width * 4, image.width * 4).copy(raw, target + 1)
  }
  const header = Buffer.alloc(13)
  header.writeUInt32BE(image.width, 0); header.writeUInt32BE(image.height, 4)
  header[8] = 8; header[9] = 6
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', header), chunk('IDAT', deflateSync(raw, { level: 9 })), chunk('IEND', Buffer.alloc(0))])
}

function save(name, image) {
  writeFileSync(resolve(assetDir, name), encodePng(image))
}

function drawPlant(kind, stage, shiny = false) {
  const image = canvas(32, 32)
  const p = shiny ? palettes.shiny : palettes[kind]
  const outline = rgba(p.outline), stem = rgba(p.stem), leaf = rgba(p.leaf), light = rgba(p.light), bloom = rgba(p.bloom)
  const heights = [6, 9, 13, 17, 21, 25]
  const top = 29 - heights[stage]
  rect(image, 14, 28, 5, 2, rgba('#4a4032', 130))
  if (kind === 'oak') {
    const trunkWidth = stage < 2 ? 1 : stage < 4 ? 2 : 3
    rect(image, 16 - Math.floor(trunkWidth / 2), top + 6, trunkWidth, 23 - top, outline)
    rect(image, 17 - Math.floor(trunkWidth / 2), top + 6, Math.max(1, trunkWidth - 1), 23 - top, stem)
    if (stage > 1) line(image, 16, top + 12, 11, top + 7, stem)
    if (stage > 2) line(image, 17, top + 11, 22, top + 6, stem)
    const radius = Math.max(2, 2 + stage)
    ellipse(image, 16, top + 6, radius + 2, Math.max(2, radius - 1), outline)
    ellipse(image, 14, top + 5, radius, Math.max(2, radius - 2), leaf)
    ellipse(image, 19, top + 7, Math.max(2, radius - 1), Math.max(2, radius - 2), light)
  } else if (kind === 'fern') {
    line(image, 16, 28, 16, top + 4, outline, 2); line(image, 16, 28, 16, top + 4, stem)
    const fronds = Math.max(1, stage + 1)
    for (let i = 0; i < fronds; i++) {
      const y = 26 - i * 3
      const reach = 3 + i + stage
      line(image, 16, y, Math.max(3, 16 - reach), Math.max(top, y - 5), outline)
      line(image, 16, y, Math.min(28, 16 + reach), Math.max(top + 1, y - 4), stem)
      for (let n = 2; n < reach; n += 3) {
        ellipse(image, 16 - n, y - Math.floor(n / 2), 2, 1, i % 2 ? light : leaf)
        ellipse(image, 16 + n, y - Math.floor(n / 2), 2, 1, i % 2 ? leaf : light)
      }
    }
  } else {
    line(image, 16, 28, 16, top + 2, outline, 2); line(image, 16, 28, 16, top + 2, stem)
    const leaves = Math.max(1, stage + 1)
    for (let i = 0; i < leaves; i++) {
      const y = 25 - i * Math.max(2, Math.floor((22 - top) / Math.max(leaves, 1)))
      const side = i % 2 ? -1 : 1
      ellipse(image, 16 + side * (3 + Math.min(i, 2)), y, 4 + Math.floor(stage / 3), 2, outline)
      ellipse(image, 16 + side * (3 + Math.min(i, 2)), y - 1, 3 + Math.floor(stage / 3), 1, i % 3 ? leaf : light)
    }
    if (kind === 'orchid' && stage >= 3) {
      const flowers = stage - 1
      for (let i = 0; i < flowers; i++) {
        const x = 16 + (i % 2 ? -3 : 3), y = top + 2 + i * 2
        ellipse(image, x, y, 2, 2, outline); setPixel(image, x - 1, y, bloom); setPixel(image, x + 1, y, bloom); setPixel(image, x, y - 1, bloom); setPixel(image, x, y + 1, bloom)
      }
    } else if (kind === 'mint' && stage >= 4) {
      sparkle(image, 16, top + 1, bloom)
    }
  }
  if (shiny) { sparkle(image, 6, 8 + stage, rgba('#fff3a6')); sparkle(image, 26, 5 + (stage % 3) * 5, rgba('#d6fff1')) }
  return image
}

function drawFox(stage, action = 'idle', frame = 0, shiny = false) {
  const image = canvas(32, 32)
  const p = shiny ? palettes.foxShiny : palettes.fox
  const outline = rgba(p.outline), body = rgba(p.body), light = rgba(p.light), cream = rgba(p.cream), dark = rgba(p.dark)
  const sizes = [5, 7, 9, 10, 11, 12]
  const size = sizes[stage]
  const sleeping = action === 'sleep'
  const eating = action === 'eat'
  const bob = action === 'move' ? frame % 2 : action === 'idle' ? Math.floor(frame / 2) : 0
  const baseY = sleeping ? 25 : 27 - bob
  if (sleeping) {
    ellipse(image, 16, 23, 9, 5, outline); ellipse(image, 15, 22, 8, 4, body); ellipse(image, 10, 22, 4, 3, light); rect(image, 18, 22, 2, 1, dark)
  } else {
    ellipse(image, 15, baseY - Math.floor(size / 3), size, Math.max(3, Math.floor(size / 2)), outline)
    ellipse(image, 15, baseY - Math.floor(size / 3) - 1, size - 1, Math.max(2, Math.floor(size / 2) - 1), body)
    const headX = eating ? 23 : 22, headY = eating ? baseY - 2 : baseY - Math.floor(size / 2) - 3
    polygon(image, [[headX - 5, headY], [headX, headY - 5], [headX + 4, headY], [headX + 2, headY + 5], [headX - 4, headY + 4]], outline)
    polygon(image, [[headX - 4, headY], [headX, headY - 4], [headX + 3, headY], [headX + 1, headY + 4], [headX - 3, headY + 3]], light)
    polygon(image, [[headX - 3, headY - 2], [headX - 1, headY - 7], [headX + 1, headY - 3]], outline)
    polygon(image, [[headX + 1, headY - 3], [headX + 3, headY - 7], [headX + 4, headY - 1]], outline)
    rect(image, headX - 2, headY + 1, 4, 2, cream); setPixel(image, headX + 2, headY + 1, dark); setPixel(image, headX, headY - 1, dark)
    const tailLift = action === 'react' ? 5 : frame % 3
    polygon(image, [[7, baseY - 4], [2, baseY - 9 - tailLift], [1, baseY - 5 - tailLift], [7, baseY], [11, baseY - 3]], outline)
    polygon(image, [[7, baseY - 4], [3, baseY - 8 - tailLift], [3, baseY - 5 - tailLift], [8, baseY - 1]], light)
    const step = action === 'move' ? (frame % 4 < 2 ? 2 : -1) : 0
    line(image, 12, baseY - 1, 11 + step, 29, outline, 2); line(image, 18, baseY - 1, 19 - step, 29, outline, 2)
  }
  if (shiny) sparkle(image, 27, 5, rgba('#fff0a0'))
  return image
}

function drawBird(stage, action = 'idle', frame = 0, shiny = false) {
  const image = canvas(32, 32)
  const p = shiny ? palettes.birdShiny : palettes.bird
  const outline = rgba(p.outline), body = rgba(p.body), light = rgba(p.light), cream = rgba(p.cream), dark = rgba(p.dark)
  const sizes = [3, 4, 5, 6, 7, 8]
  const size = sizes[stage]
  const wing = action === 'fly' ? [0, -4, -1, 3, 0, -3][frame % 6] : action === 'react' ? -2 : 1
  const y = 17 + (action === 'idle' ? frame % 2 : 0)
  ellipse(image, 16, y, size, Math.max(3, Math.floor(size * 0.65)), outline)
  ellipse(image, 16, y - 1, Math.max(2, size - 1), Math.max(2, Math.floor(size * 0.5)), body)
  polygon(image, [[14, y], [8, y + wing], [10, y + 5], [17, y + 2]], outline)
  polygon(image, [[14, y], [9, y + wing], [11, y + 3], [17, y + 1]], light)
  ellipse(image, 21, y - 3, Math.max(2, Math.floor(size / 2)), Math.max(2, Math.floor(size / 2)), outline)
  ellipse(image, 21, y - 4, Math.max(1, Math.floor(size / 2) - 1), Math.max(1, Math.floor(size / 2) - 1), light)
  polygon(image, [[23, y - 3], [28, y - 1], [23, y]], cream)
  setPixel(image, 22, y - 5, dark)
  polygon(image, [[10, y + 1], [4, y - 1], [8, y + 4]], outline)
  if (shiny) sparkle(image, 7, 7, rgba('#fff1a2'))
  return image
}

function drawScene(night = false) {
  const image = canvas(120, 40, rgba(night ? '#16223d' : '#8fd0d9'))
  const skyBands = night ? ['#16223d', '#203354', '#2d4662', '#405f70'] : ['#8fd0d9', '#a6d9d7', '#c7e3d2', '#f2e4ba']
  skyBands.forEach((color, index) => rect(image, 0, index * 5, 120, 6, rgba(color)))
  if (night) {
    ellipse(image, 92, 7, 4, 4, rgba('#f5e8b2')); ellipse(image, 94, 6, 4, 4, rgba('#203354'))
    for (let i = 0; i < 28; i++) setPixel(image, (i * 37) % 118 + 1, (i * 17) % 14 + 1, rgba(i % 4 ? '#a9c4d6' : '#f6dda0'))
  } else {
    ellipse(image, 94, 8, 5, 5, rgba('#f6d46b')); rect(image, 12, 6, 12, 2, rgba('#eef7e9')); rect(image, 15, 4, 7, 4, rgba('#eef7e9'))
  }
  polygon(image, [[0, 22], [16, 15], [29, 21], [46, 14], [64, 22], [83, 16], [101, 22], [120, 17], [120, 40], [0, 40]], rgba(night ? '#263e4a' : '#7daa78'))
  polygon(image, [[0, 25], [21, 20], [39, 24], [61, 18], [81, 24], [104, 20], [120, 23], [120, 40], [0, 40]], rgba(night ? '#2f514c' : '#6e9c68'))
  rect(image, 0, 26, 120, 14, rgba(night ? '#29483f' : '#5e8d55'))
  for (let x = 0; x < 120; x += 3) {
    const color = (x / 3) % 2 ? (night ? '#34594a' : '#6c9d59') : (night ? '#244137' : '#527f49')
    rect(image, x, 29 + ((x * 7) % 4), 2, 8, rgba(color))
  }
  polygon(image, [[52, 40], [58, 26], [64, 26], [70, 40]], rgba(night ? '#65594d' : '#b79b6b'))
  polygon(image, [[56, 40], [60, 27], [63, 27], [66, 40]], rgba(night ? '#7c6d5c' : '#d5bb82'))
  for (const x of [8, 32, 108]) {
    rect(image, x, 20, 2, 8, rgba(night ? '#182f32' : '#4f6647'))
    polygon(image, [[x - 5, 22], [x + 1, 14], [x + 7, 22]], rgba(night ? '#1d3938' : '#496f4d'))
    polygon(image, [[x - 4, 19], [x + 1, 12], [x + 6, 19]], rgba(night ? '#23433f' : '#5d8256'))
  }
  if (night) for (let i = 0; i < 12; i++) setPixel(image, 5 + (i * 29) % 110, 27 + (i * 11) % 10, rgba(i % 3 ? '#9ecbb0' : '#e9d977'))
  return scaleNearest(image, 8)
}

save('scene-day.png', drawScene(false))
save('scene-night.png', drawScene(true))

const plantKinds = ['mint', 'fern', 'orchid', 'oak']
for (const kind of plantKinds) for (let stage = 0; stage < 6; stage++) {
  save(`${kind}-${stage}.png`, scaleNearest(drawPlant(kind, stage, false), 2))
  save(`${kind}-${stage}-shiny.png`, scaleNearest(drawPlant(kind, stage, true), 2))
}

for (let stage = 0; stage < 6; stage++) {
  save(`fox-${stage}.png`, scaleNearest(drawFox(stage), 2))
  save(`fox-${stage}-shiny.png`, scaleNearest(drawFox(stage, 'idle', 0, true), 2))
  save(`bird-${stage}.png`, scaleNearest(drawBird(stage), 2))
  save(`bird-${stage}-shiny.png`, scaleNearest(drawBird(stage, 'idle', 0, true), 2))
}

const foxClips = { idle: 4, move: 8, eat: 6, sleep: 4, react: 4 }
for (const [action, frames] of Object.entries(foxClips)) save(`fox-${action}.png`, concatFrames(Array.from({ length: frames }, (_, frame) => drawFox(5, action, frame))))
const birdClips = { idle: 4, fly: 6, react: 4 }
for (const [action, frames] of Object.entries(birdClips)) save(`bird-${action}.png`, concatFrames(Array.from({ length: frames }, (_, frame) => drawBird(5, action, frame))))

const files = [
  'scene-day.png', 'scene-night.png',
  ...plantKinds.flatMap(kind => Array.from({ length: 6 }, (_, stage) => [`${kind}-${stage}.png`, `${kind}-${stage}-shiny.png`]).flat()),
  ...Array.from({ length: 6 }, (_, stage) => [`fox-${stage}.png`, `fox-${stage}-shiny.png`, `bird-${stage}.png`, `bird-${stage}-shiny.png`]).flat(),
  ...Object.keys(foxClips).map(action => `fox-${action}.png`),
  ...Object.keys(birdClips).map(action => `bird-${action}.png`),
]
const imports = files.map((file, index) => `import asset${index} from './assets/${file}'`).join('\n')
const entries = files.map((file, index) => `  'assets/${file}': asset${index},`).join('\n')
writeFileSync(resolve(packDir, 'assets.generated.ts'), `${imports}\n\nexport const pixelHabitatSources: Record<string, string> = {\n${entries}\n}\n`)

function pngDimensions(name) {
  const buffer = readFileSync(resolve(assetDir, name))
  return { width: buffer.readUInt32BE(16), height: buffer.readUInt32BE(20) }
}

const assets = Object.fromEntries(files.map(file => {
  const id = file.replace(/\.png$/, '')
  const { width, height } = pngDimensions(file)
  const animated = file.startsWith('fox-') && Object.hasOwn(foxClips, id.slice(4))
    || file.startsWith('bird-') && Object.hasOwn(birdClips, id.slice(5))
  return [id, { path: `assets/${file}`, kind: animated ? 'sprite-sheet' : 'image', bytes: statSync(resolve(assetDir, file)).size, width, height }]
}))

const six = (prefix, shiny = false) => Array.from({ length: 6 }, (_, stage) => `${prefix}-${stage}${shiny ? '-shiny' : ''}`)
const still = { format: 'procedural', primitive: 'still', durationMs: 6000, loop: true }
const sway = durationMs => ({ format: 'procedural', primitive: 'sway', durationMs, loop: true })
const sprite = (asset, frames, fps, loop = true, next) => ({ format: 'sprite-sheet', asset, frameWidth: 64, frameHeight: 64, frames, columns: frames, rows: 1, fps, loop, staticFrame: 0, flipX: true, ...(next ? { next } : {}) })
const plant = ({ name, latin, meaning, description, prefix, layer, durationMs }) => ({
  name, latin, meaning, description,
  stageLabels: ['嫩芽', '幼株', '展叶', '繁茂', '初花', '盛放'],
  stages: six(prefix), shinyStages: six(prefix, true),
  anchor: { x: 0.5, y: 0.94 }, hitArea: { x: 0.08, y: 0.02, width: 0.84, height: 0.96 }, defaultSize: 0.72,
  placement: { type: 'ambient', zone: 'meadow', layer },
  actions: { idle: sway(durationMs) },
  behavior: { durations: { idle: { minMs: durationMs, maxMs: durationMs } }, weights: { idle: 1 } },
  reducedMotion: { asset: `${prefix}-5` },
})

const manifest = {
  contractVersion: 2,
  id: 'ops-pixel-habitat',
  version: '1.0.0',
  name: '像素栖息地',
  author: 'OpsCopilot',
  art: { style: 'pixel', baseGrid: 32, subjectSize: 64 },
  runtime: { min: 2, max: 2 },
  stateSchema: { min: 2, max: 2 },
  presentation: {
    title: '栖息地', unit: '位伙伴',
    empty: '完成一次可靠的工作，让第一位伙伴来到这里。',
    maxLevel: '相伴',
  },
  stageLabels: ['初生', '成长', '成形', '进阶', '焕新', '相伴'],
  assets,
  scene: {
    dayAsset: 'scene-day', nightAsset: 'scene-night', aspectRatio: 3, fit: 'stretch',
    safeArea: { x: 0.03, y: 0.05, width: 0.94, height: 0.92 },
    zones: {
      meadow: { kind: 'ground', polygon: [{ x: 0.04, y: 0.58 }, { x: 0.96, y: 0.58 }, { x: 0.96, y: 0.96 }, { x: 0.04, y: 0.96 }] },
      sky: { kind: 'air', polygon: [{ x: 0.06, y: 0.12 }, { x: 0.94, y: 0.12 }, { x: 0.94, y: 0.5 }, { x: 0.06, y: 0.5 }] },
    },
    layers: ['far', 'middle', 'near', 'foreground'],
    slots: [
      { x: 0.12, y: 0.88, size: 0.7, zone: 'meadow', layer: 'middle' },
      { x: 0.3, y: 0.81, size: 0.66, zone: 'meadow', layer: 'far' },
      { x: 0.5, y: 0.9, size: 0.74, zone: 'meadow', layer: 'near' },
      { x: 0.7, y: 0.82, size: 0.68, zone: 'meadow', layer: 'middle' },
      { x: 0.88, y: 0.91, size: 0.78, zone: 'meadow', layer: 'foreground' },
      { x: 0.28, y: 0.3, size: 0.42, zone: 'sky', layer: 'near' },
      { x: 0.76, y: 0.24, size: 0.38, zone: 'sky', layer: 'far' },
    ],
    decorations: [],
    lighting: { nightBrightness: 0.72, nightSaturation: 0.82 },
  },
  elements: {
    'cmd-mint': plant({ name: '命令薄荷', latin: 'Mentha pixelata', meaning: '把重复工作变得轻巧', description: '每一次可靠复用，都会添上一片清爽的新叶。', prefix: 'mint', layer: 'middle', durationMs: 9600 }),
    'transfer-fern': plant({ name: '传输蕨', latin: 'Filix transitus', meaning: '信息在环境间安全流动', description: '舒展的羽叶，记录每一次完整抵达的传输。', prefix: 'fern', layer: 'near', durationMs: 11200 }),
    'guard-orchid': plant({ name: '守护兰', latin: 'Orchis tutela', meaning: '克制、边界与安全操作', description: '在被认真守住的边界旁，花枝安静地亮起。', prefix: 'orchid', layer: 'near', durationMs: 10400 }),
    'knowledge-tree': plant({ name: '知识橡树', latin: 'Quercus memoria', meaning: '让一次排障成为团队的长期记忆', description: '年轮收下答案，也收下寻找答案时走过的路。', prefix: 'oak', layer: 'far', durationMs: 13800 }),
    'session-tree': {
      name: '会话狐', latin: 'Vulpes terminalis', meaning: '稳定建立连接，也懂得适时休息', description: '它沿着安全的路径巡游，记得每一次可靠建立的会话。',
      stageLabels: ['幼崽', '探步', '巡游', '敏捷', '领地', '相伴'], stages: six('fox'), shinyStages: six('fox', true), stageScales: [0.56, 0.66, 0.76, 0.86, 0.94, 1],
      anchor: { x: 0.5, y: 0.94 }, hitArea: { x: 0.04, y: 0.12, width: 0.92, height: 0.86 }, defaultSize: 0.8,
      placement: { type: 'ground-roaming', zone: 'meadow', layer: 'foreground' },
      actions: { idle: still, move: sprite('fox-move', 8, 8), eat: sprite('fox-eat', 6, 6, false, 'idle'), sleep: sprite('fox-sleep', 4, 3), react: sprite('fox-react', 4, 8, false, 'idle') },
      behavior: {
        durations: { idle: { minMs: 2800, maxMs: 7200 }, move: { minMs: 2200, maxMs: 5200 }, eat: { minMs: 1000, maxMs: 2200 }, sleep: { minMs: 4200, maxMs: 9000 }, react: { minMs: 500, maxMs: 900 } },
        weights: { idle: 42, move: 34, eat: 14, sleep: 10 }, speed: { min: 0.025, max: 0.052 }, avoidanceRadius: 0.08,
      },
      reducedMotion: { asset: 'fox-5' },
    },
    'script-vine': {
      name: '脚本云雀', latin: 'Alauda automata', meaning: '让自动化替你飞过重复的路', description: '当脚本顺利跑完，它就在天空里画出一段新的航线。',
      stageLabels: ['雏羽', '试翼', '短飞', '巡空', '展翼', '远航'], stages: six('bird'), shinyStages: six('bird', true), stageScales: [0.52, 0.64, 0.74, 0.84, 0.93, 1],
      anchor: { x: 0.5, y: 0.62 }, hitArea: { x: 0.05, y: 0.06, width: 0.9, height: 0.82 }, defaultSize: 0.62,
      placement: { type: 'air-roaming', zone: 'sky', layer: 'near' },
      actions: { idle: still, fly: sprite('bird-fly', 6, 9), react: sprite('bird-react', 4, 8, false, 'idle') },
      behavior: {
        durations: { idle: { minMs: 1800, maxMs: 4600 }, fly: { minMs: 2600, maxMs: 6200 }, react: { minMs: 500, maxMs: 900 } },
        weights: { idle: 38, fly: 62 }, speed: { min: 0.035, max: 0.068 }, avoidanceRadius: 0.06,
      },
      reducedMotion: { asset: 'bird-5' },
    },
  },
}

writeFileSync(resolve(packDir, 'manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`)

console.log(`generated ${files.length} pixel assets in ${relative(repoRoot, assetDir)}`)

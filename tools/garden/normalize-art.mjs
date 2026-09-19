/**
 * 养成素材批量归一化：把绘制稿的九宫格切成单张素材，按定稿配方统一处理。
 *
 * 配方取自 frontend-shell/garden-recipe.mjs——与调优台是同一份实现，
 * 这里的参数必须与 docs/garden-presentation-architecture.md 第 4.2 节的配方表一致。
 *
 * 用法：
 *   node tools/garden/normalize-art.mjs [--src DIR] [--out DIR] [--dry]
 *
 * 输入命名（绘制方按提示词卡片标题存盘即可）：
 *   <元素名>.png           普通品质九宫格
 *   <元素名> · 闪光形态.png  闪光品质九宫格
 *   <主题名> · 白天场景.png / <主题名> · 夜晚场景.png   整幅场景（可选）
 *
 * 输出：默认直接写回内容包目录 frontend-shell/src/ui/garden/packs/pixelHabitat/assets/
 * （每个元素每品质 9 张 64×64：6 阶段 + 3 个额外成熟变体），
 * 命名为 <前缀>-<阶段>.png 与 <前缀>-mature-<b|c|d>.png，闪光加 -shiny 后缀。
 */
import { readFileSync, writeFileSync, mkdirSync, existsSync, rmSync } from 'node:fs'
import { join, resolve } from 'node:path'

// 配方的实现依赖浏览器 ImageData；Node 里没有，先垫一个同形状的最小实现。
if (typeof globalThis.ImageData === 'undefined') {
  globalThis.ImageData = class ImageData {
    constructor(width, height) {
      this.width = width
      this.height = height
      this.data = new Uint8ClampedArray(width * height * 4)
    }
  }
}

const { decodePng, encodePng, cropImage } = await import('./png.mjs')
const recipe = await import('../../frontend-shell/garden-recipe.mjs')

/* 配方参数：与调优台默认值、文档配方表三处必须一致 */
const RECIPE = {
  outW: 64, outH: 64, fitContain: false,
  preScale: 50,          // 源图先缩到 50% 再采样
  preColors: 24,         // 源图预量化色数，实测 24 最干净
  bgMode: 'corners', bgTol: 36, bgLuma: 228,
  alphaCut: 128, alphaBinarize: true,
  quantize: true, colors: 96, dither: 'none', ditherAmt: 60,
  outline: 'auto', outlineW: 1, outlineDark: 60, outlineColor: '#1a1420',
}

const ELEMENTS = [
  { key: 'cmd-mint', prefix: 'mint', name: '命令薄荷' },
  { key: 'transfer-fern', prefix: 'fern', name: '传输蕨' },
  { key: 'guard-orchid', prefix: 'orchid', name: '守护兰' },
  { key: 'knowledge-tree', prefix: 'oak', name: '知识橡树' },
  { key: 'session-tree', prefix: 'fungus', name: '会话菌' },
  { key: 'script-vine', prefix: 'vine', name: '脚本藤' },
]

/* 场景：3:1 横版背景，输出 960×320；不描边（它是底图，不是主体） */
const SCENES = [
  { asset: 'scene-day', file: '场景 · 白天.png', width: 960, height: 320 },
  { asset: 'scene-night', file: '场景 · 夜晚.png', width: 960, height: 320 },
]

const QUALITIES = [
  { id: 'normal', suffix: '', file: name => `${name}.png` },
  { id: 'shiny', suffix: '-shiny', file: name => `${name} · 闪光形态.png` },
]

/* 九宫格 → 素材的对应：前 6 格是六个阶段，第 6–9 格是成熟形态的变体池 */
const CELL_ASSETS = [
  { cell: 0, asset: prefix => `${prefix}-0` },
  { cell: 1, asset: prefix => `${prefix}-1` },
  { cell: 2, asset: prefix => `${prefix}-2` },
  { cell: 3, asset: prefix => `${prefix}-3` },
  { cell: 4, asset: prefix => `${prefix}-4` },
  { cell: 5, asset: prefix => `${prefix}-5` },
  { cell: 6, asset: prefix => `${prefix}-mature-b` },
  { cell: 7, asset: prefix => `${prefix}-mature-c` },
  { cell: 8, asset: prefix => `${prefix}-mature-d` },
]

function parseArgs(argv) {
  const value = name => {
    const index = argv.indexOf(name)
    return index >= 0 && argv[index + 1] ? argv[index + 1] : undefined
  }
  return {
    src: value('--src') ?? 'C:\\Users\\15802\\Pictures\\ops',
    out: value('--out') ?? resolve(import.meta.dirname, '..', '..', 'frontend-shell', 'src', 'ui', 'garden', 'packs', 'pixelHabitat', 'assets'),
    dry: argv.includes('--dry'),
  }
}

/** 源图预缩：整幅缩到 50%，之后才裁格——顺序与文档配方一致。 */
function preScale(image, factor) {
  const width = Math.max(1, Math.round(image.width * factor))
  const height = Math.max(1, Math.round(image.height * factor))
  const place = { x: 0, y: 0, w: width, h: height, sx: 0, sy: 0, sw: image.width, sh: image.height }
  return recipe.resample(image, width, height, place, 'box')
}

/** 对一个格子跑完整配方，返回 64×64 的 ImageData。 */
function normalizeCell(cell) {
  const p = RECIPE
  const src = { width: cell.width, height: cell.height, data: new Uint8ClampedArray(cell.data) }
  if (p.bgMode !== 'none') recipe.removeBackground(src, p)
  if (p.preColors > 0) recipe.quantizeTo(src, p.preColors, 'none', 0)
  const place = recipe.fitPlacement(src.width, src.height, p.outW, p.outH, p.fitContain)
  const out = recipe.resample(src, p.outW, p.outH, place, 'mode')
  recipe.applyAlpha(out, p)
  if (p.quantize) recipe.quantizeImage(out, p)
  return p.outline === 'none' ? out : recipe.addOutline(out, p)
}

/**
 * 场景归一化：与主体同一套预处理，但输出整幅尺寸且不描边。
 * 场景是底图，描边会在画面边缘多出一圈深色，不能加。
 */
function normalizeScene(image) {
  const p = RECIPE
  const src = { width: image.width, height: image.height, data: new Uint8ClampedArray(image.data) }
  if (p.preColors > 0) recipe.quantizeTo(src, p.preColors, 'none', 0)
  const width = image.sceneWidth
  const height = image.sceneHeight
  const place = recipe.fitPlacement(src.width, src.height, width, height, false)
  const out = recipe.resample(src, width, height, place, 'mode')
  recipe.quantizeTo(out, p.colors, 'none', 0)
  return out
}

function stats(image) {
  let opaque = 0
  for (let i = 3; i < image.data.length; i += 4) if (image.data[i] >= 128) opaque++
  return opaque / (image.width * image.height)
}

function main() {
  const { src, out, dry } = parseArgs(process.argv.slice(2))
  if (!existsSync(src)) throw new Error(`源目录不存在：${src}`)
  console.log(`源目录：${src}`)
  console.log(`输出目录：${out}`)
  if (!dry) {
    rmSync(out, { recursive: true, force: true })
    mkdirSync(out, { recursive: true })
  }

  const written = []
  const missing = []
  let skipped = 0
  for (const element of ELEMENTS) {
    for (const quality of QUALITIES) {
      const file = join(src, quality.file(element.name))
      if (!existsSync(file)) {
        missing.push(quality.file(element.name))
        continue
      }
      const source = decodePng(readFileSync(file))
      const half = preScale(source, RECIPE.preScale / 100)
      const cellWidth = Math.floor(half.width / 3)
      const cellHeight = Math.floor(half.height / 3)
      for (const mapping of CELL_ASSETS) {
        const col = mapping.cell % 3
        const row = Math.floor(mapping.cell / 3)
        const cell = cropImage(half, col * cellWidth, row * cellHeight, cellWidth, cellHeight)
        const result = normalizeCell(cell)
        const coverage = stats(result)
        const name = `${mapping.asset(element.prefix)}${quality.suffix}.png`
        if (coverage < 0.005) {
          skipped++
          console.log(`  ! ${name} 内容几乎为空（覆盖率 ${(coverage * 100).toFixed(1)}%），已跳过`)
          continue
        }
        const encoded = encodePng(result)
        if (!dry) writeFileSync(join(out, name), encoded)
        written.push({ name, coverage, bytes: encoded.length })
      }
    }
  }

  for (const scene of SCENES) {
    const file = join(src, scene.file)
    if (!existsSync(file)) {
      missing.push(scene.file)
      continue
    }
    const source = decodePng(readFileSync(file))
    source.sceneWidth = scene.width
    source.sceneHeight = scene.height
    const encoded = encodePng(normalizeScene(source))
    if (!dry) writeFileSync(join(out, `${scene.asset}.png`), encoded)
    written.push({ name: `${scene.asset}.png`, coverage: 1, bytes: encoded.length })
  }

  const totalBytes = written.reduce((sum, item) => sum + item.bytes, 0)
  console.log(`\n写出 ${written.length} 张素材${dry ? '（dry run，未落盘）' : `，共 ${(totalBytes / 1024).toFixed(0)} KB`}`)
  if (skipped) console.log(`跳过 ${skipped} 张（内容为空）`)
  if (missing.length) {
    console.log(`\n缺少 ${missing.length} 张绘制稿：`)
    for (const name of missing) console.log(`  - ${name}`)
  }
  const sizes = written.map(item => item.bytes)
  if (sizes.length) {
    console.log(`单张体积：最小 ${(Math.min(...sizes) / 1024).toFixed(1)} KB / 最大 ${(Math.max(...sizes) / 1024).toFixed(1)} KB`)
  }
}

main()

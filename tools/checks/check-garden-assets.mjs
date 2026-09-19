// Build gate for declarative cultivation content packs.
// It verifies that manifest metadata matches real files and that frame animation declarations
// fit their sprite sheets before desktop or Teams bundles are produced.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve, sep } from 'node:path'

const repoRoot = resolve(import.meta.dirname, '..', '..')
const rootArgument = process.argv.find(value => value.startsWith('--packs-root='))
const packsRoot = rootArgument ? resolve(rootArgument.slice('--packs-root='.length)) : join(repoRoot, 'frontend-shell', 'src', 'ui', 'garden', 'packs')
const MAX_PACK_BYTES = 12 * 1024 * 1024
const MAX_IMAGE_BYTES = 2 * 1024 * 1024
const MAX_ANIMATION_BYTES = 4 * 1024 * 1024
const MAX_FRAMES = 120
const MAX_FPS = 24

function fail(message) {
  throw new Error(`garden asset gate: ${message}`)
}

function* manifests(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) yield* manifests(path)
    else if (entry.name === 'manifest.json') yield path
  }
}

function uint24LE(buffer, offset) {
  return buffer[offset] | (buffer[offset + 1] << 8) | (buffer[offset + 2] << 16)
}

function dimensions(buffer, path) {
  if (buffer.length >= 24 && buffer.subarray(1, 4).toString('ascii') === 'PNG') {
    return { width: buffer.readUInt32BE(16), height: buffer.readUInt32BE(20) }
  }
  if (buffer.length < 30 || buffer.subarray(0, 4).toString('ascii') !== 'RIFF' || buffer.subarray(8, 12).toString('ascii') !== 'WEBP') fail(`${path} 不是有效的 PNG/WebP`)
  const chunk = buffer.subarray(12, 16).toString('ascii')
  if (chunk === 'VP8X') return { width: uint24LE(buffer, 24) + 1, height: uint24LE(buffer, 27) + 1 }
  if (chunk === 'VP8L') {
    if (buffer[20] !== 0x2f) fail(`${path} 的 VP8L 头无效`)
    return {
      width: 1 + buffer[21] + ((buffer[22] & 0x3f) << 8),
      height: 1 + ((buffer[22] & 0xc0) >> 6) + (buffer[23] << 2) + ((buffer[24] & 0x0f) << 10),
    }
  }
  if (chunk === 'VP8 ') {
    if (buffer[23] !== 0x9d || buffer[24] !== 0x01 || buffer[25] !== 0x2a) fail(`${path} 的 VP8 帧头无效`)
    return { width: buffer.readUInt16LE(26) & 0x3fff, height: buffer.readUInt16LE(28) & 0x3fff }
  }
  fail(`${path} 使用了不支持的 WebP 块 ${chunk}`)
}

function checkManifest(path) {
  const packDir = dirname(path)
  const manifest = JSON.parse(readFileSync(path, 'utf8'))
  const label = `${manifest.id ?? relative(repoRoot, path)}@${manifest.version ?? '?'}`
  if (manifest.contractVersion !== 2) fail(`${label} 契约版本不是 2`)
  if (!manifest.assets || !Object.keys(manifest.assets).length) fail(`${label} 没有资源`)
  let total = 0
  const actual = new Map()
  for (const [id, asset] of Object.entries(manifest.assets)) {
    const target = resolve(packDir, asset.path)
    const rel = relative(packDir, target)
    if (!rel || rel.startsWith(`..${sep}`) || rel === '..') fail(`${label}/${id} 路径越出内容包`)
    const size = statSync(target).size
    const buffer = readFileSync(target)
    const measured = dimensions(buffer, relative(repoRoot, target))
    if (size !== asset.bytes) fail(`${label}/${id} bytes 声明 ${asset.bytes}，实际 ${size}`)
    if (measured.width !== asset.width || measured.height !== asset.height) fail(`${label}/${id} 尺寸声明 ${asset.width}x${asset.height}，实际 ${measured.width}x${measured.height}`)
    const limit = asset.kind === 'sprite-sheet' ? MAX_ANIMATION_BYTES : MAX_IMAGE_BYTES
    if (size > limit) fail(`${label}/${id} 超过单资源预算 ${limit}`)
    total += size
    actual.set(id, { ...asset, ...measured })
  }
  if (total > MAX_PACK_BYTES) fail(`${label} 总资源 ${total} 超过预算 ${MAX_PACK_BYTES}`)

  if (manifest.art?.style === 'pixel') {
    if (manifest.art.baseGrid !== 32 || manifest.art.subjectSize !== 64) fail(`${label} 像素主题必须使用 32px 基础网格与 64px 主体尺寸`)
    for (const [id, asset] of actual) {
      if (asset.width % manifest.art.baseGrid !== 0 || asset.height % manifest.art.baseGrid !== 0) fail(`${label}/${id} 尺寸必须对齐 ${manifest.art.baseGrid}px 网格`)
    }
  }

  for (const [itemId, element] of Object.entries(manifest.elements ?? {})) {
    if (!Array.isArray(element.stages) || element.stages.length !== 6) fail(`${label}/${itemId} 必须声明六阶段素材`)
    if (manifest.art?.style === 'pixel') {
      for (const assetId of [...element.stages, ...(element.shinyStages ?? [])]) {
        const asset = actual.get(assetId)
        if (!asset || asset.width !== manifest.art.subjectSize || asset.height !== manifest.art.subjectSize) fail(`${label}/${itemId}/${assetId} 主体素材必须是 ${manifest.art.subjectSize}x${manifest.art.subjectSize}px`)
      }
    }
    for (const [actionName, clip] of Object.entries(element.actions ?? {})) {
      if (clip.format === 'sprite-sheet') {
        const asset = actual.get(clip.asset)
        if (!asset || asset.kind !== 'sprite-sheet') fail(`${label}/${itemId}/${actionName} 图集资源不存在或类型错误`)
        if (clip.frames < 1 || clip.frames > MAX_FRAMES || clip.fps < 1 || clip.fps > MAX_FPS) fail(`${label}/${itemId}/${actionName} 帧数或帧率越界`)
        if (clip.frames > clip.columns * clip.rows || clip.columns * clip.frameWidth > asset.width || clip.rows * clip.frameHeight > asset.height) fail(`${label}/${itemId}/${actionName} 帧网格超出图集`)
        if (manifest.art?.style === 'pixel' && (clip.frameWidth % manifest.art.baseGrid !== 0 || clip.frameHeight % manifest.art.baseGrid !== 0 || clip.frameWidth > 96 || clip.frameHeight > 96)) fail(`${label}/${itemId}/${actionName} 单帧必须按 32px 网格制作，且边长不超过 96px`)
      } else if (clip.format === 'frame-sequence') {
        if (!Array.isArray(clip.assets) || clip.assets.length < 1 || clip.assets.length > MAX_FRAMES || clip.fps < 1 || clip.fps > MAX_FPS) fail(`${label}/${itemId}/${actionName} 帧序列规格越界`)
        for (const assetId of clip.assets) if (!actual.has(assetId)) fail(`${label}/${itemId}/${actionName} 引用了不存在的帧 ${assetId}`)
      }
    }
  }
  return { label, assets: actual.size, bytes: total }
}

const paths = [...manifests(packsRoot)]
if (!paths.length) fail('没有找到 manifest.json')
for (const result of paths.map(checkManifest)) console.log(`garden asset gate: ${result.label} ${result.assets} assets ${result.bytes} bytes`)

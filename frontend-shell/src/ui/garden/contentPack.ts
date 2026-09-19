export const GARDEN_PACK_CONTRACT_VERSION = 2
export const GARDEN_RUNTIME_VERSION = 2

export const MAX_PACK_ASSET_BYTES = 12 * 1024 * 1024
export const MAX_IMAGE_ASSET_BYTES = 2 * 1024 * 1024
export const MAX_ANIMATION_ASSET_BYTES = 4 * 1024 * 1024
export const MAX_ANIMATION_FRAMES = 120
export const MAX_ANIMATION_FPS = 24
export const MAX_FRAME_EDGE = 1024
export const MAX_ACTIVE_GARDEN_ACTORS = 12

export type GardenPlacement = 'anchored' | 'ambient' | 'ground-roaming' | 'air-roaming'
export type GardenActionName = 'idle' | 'move' | 'eat' | 'sleep' | 'hop' | 'fly' | 'react'
export type GardenAutonomousAction = Exclude<GardenActionName, 'react'>
export type GardenAssetKind = 'image' | 'sprite-sheet'

export interface GardenVersionRange {
  min: number
  max: number
}

export interface GardenAssetManifest {
  path: string
  kind: GardenAssetKind
  bytes: number
  width: number
  height: number
}

export interface GardenAsset extends GardenAssetManifest {
  id: string
  src: string
}

export interface GardenArtManifest {
  style: 'pixel'
  baseGrid: 32
  subjectSize: 64
}

export interface GardenPoint {
  x: number
  y: number
}

export interface GardenRect extends GardenPoint {
  width: number
  height: number
}

export interface GardenSceneZone {
  kind: 'ground' | 'air' | 'water'
  polygon: GardenPoint[]
}

export interface GardenSceneSlot extends GardenPoint {
  size: number
  zone: string
  layer: string
}

export interface GardenSceneDecoration extends GardenSceneSlot {
  asset: string
  anchor?: GardenPoint
}

export interface GardenSceneManifest {
  /** Fit keeps the entire authored composition visible without stretching or cropping. */
  framing?: 'fit'
  ambience?: { kind: 'mote' | 'ripple'; x: number; y: number; size: number; durationMs: number; delayMs: number }[]
  dayAsset: string
  nightAsset: string
  aspectRatio: number
  safeArea: GardenRect
  fit: 'cover' | 'contain' | 'stretch'
  zones: Record<string, GardenSceneZone>
  layers: string[]
  slots: GardenSceneSlot[]
  decorations: GardenSceneDecoration[]
  /** Package-authored background tone; keeps dense scenery subordinate to collectibles. */
  backdrop?: {
    day: { brightness: number; saturation: number; contrast: number; opacity: number }
    night: { brightness: number; saturation: number; contrast: number; opacity: number }
  }
  lighting: {
    nightBrightness: number
    nightSaturation: number
  }
}

export interface GardenSpriteSheetClip {
  format: 'sprite-sheet'
  asset: string
  frameWidth: number
  frameHeight: number
  frames: number
  columns: number
  rows: number
  fps: number
  loop: boolean
  staticFrame: number
  flipX?: boolean
  next?: GardenActionName
}

export interface GardenFrameSequenceClip {
  format: 'frame-sequence'
  assets: string[]
  fps: number
  loop: boolean
  staticFrame: number
  flipX?: boolean
  next?: GardenActionName
}

export interface GardenProceduralClip {
  format: 'procedural'
  primitive: 'sway' | 'still'
  durationMs: number
  loop: boolean
}

export type GardenActionClip = GardenSpriteSheetClip | GardenFrameSequenceClip | GardenProceduralClip

export interface GardenDurationRange {
  minMs: number
  maxMs: number
}

export interface GardenNumberRange {
  min: number
  max: number
}

/**
 * 行为参数与动作素材分离。运行时只解释这些通用参数，内容包不能携带行为代码。
 * speed 使用场景归一化坐标/秒；avoidanceRadius 使用场景归一化坐标。
 */
export interface GardenBehaviorManifest {
  durations: Partial<Record<GardenActionName, GardenDurationRange>>
  weights: Partial<Record<GardenAutonomousAction, number>>
  speed?: GardenNumberRange
  avoidanceRadius?: number
}

export interface GardenElementManifest {
  name: string
  meaning: string
  latin?: string
  description?: string
  /** Optional commerce metadata. Without it the element can render but is not sold. */
  commerce?: { price: number; category: string; initialLevel: number }
  stageLabels?: string[]
  stages: string[]
  shinyStages?: string[]
  matureVariants?: string[]
  shinyMatureVariants?: string[]
  anchor: GardenPoint
  hitArea: GardenRect
  defaultSize: number
  stageScales?: number[]
  placement: {
    type: GardenPlacement
    zone: string
    layer: string
  }
  actions: Partial<Record<GardenActionName, GardenActionClip>>
  behavior: GardenBehaviorManifest
  reducedMotion: {
    asset: string
    frame?: number
  }
}

export interface GardenPackManifest {
  contractVersion: number
  id: string
  version: string
  name: string
  author?: string
  art?: GardenArtManifest
  runtime: GardenVersionRange
  stateSchema: GardenVersionRange
  presentation: {
    title: string
    unit: string
    empty: string
    maxLevel: string
  }
  stageLabels: string[]
  assets: Record<string, GardenAssetManifest>
  scene: GardenSceneManifest
  elements: Record<string, GardenElementManifest>
}

export interface GardenPack extends Omit<GardenPackManifest, 'assets'> {
  assets: Record<string, GardenAsset>
  appliedOverlays?: string[]
}

export interface GardenOverlayElement {
  name?: string
  meaning?: string
  description?: string
  stages?: string[]
  shinyStages?: string[]
  matureVariants?: string[]
  shinyMatureVariants?: string[]
  reducedMotion?: { asset: string; frame?: number }
}

export interface GardenOverlayManifest {
  contractVersion: number
  id: string
  version: string
  name: string
  targetPackIds: string[]
  runtime: GardenVersionRange
  assets: Record<string, GardenAssetManifest>
  scene?: Partial<Pick<GardenSceneManifest, 'dayAsset' | 'nightAsset' | 'decorations'>> & {
    lighting?: Partial<GardenSceneManifest['lighting']>
  }
  elements?: Record<string, GardenOverlayElement>
}

export interface GardenOverlay extends Omit<GardenOverlayManifest, 'assets'> {
  assets: Record<string, GardenAsset>
}

const ID_PATTERN = /^[a-z0-9]+(?:[.-][a-z0-9]+)*$/
const VERSION_PATTERN = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/
const RELATIVE_ASSET_PATTERN = /^(?![A-Za-z]:)(?![/\\])(?!.*(?:^|[/\\])\.\.(?:[/\\]|$)).+\.(?:png|webp)$/i
const ACTIONS: readonly GardenActionName[] = ['idle', 'move', 'eat', 'sleep', 'hop', 'fly', 'react']
const PLACEMENTS: readonly GardenPlacement[] = ['anchored', 'ambient', 'ground-roaming', 'air-roaming']

function fail(label: string, message: string): never {
  throw new Error(`${label}：${message}`)
}

function finite(value: number, label: string): void {
  if (!Number.isFinite(value)) fail(label, '必须是有限数值')
}

function normalized(value: number, label: string): void {
  finite(value, label)
  if (value < 0 || value > 1) fail(label, '必须位于 0–1')
}

function nonEmpty(value: string, label: string): void {
  if (typeof value !== 'string' || value.trim() === '') fail(label, '不能为空')
}

function range(value: GardenVersionRange, label: string): void {
  if (!Number.isInteger(value?.min) || !Number.isInteger(value?.max) || value.min < 1 || value.max < value.min) {
    fail(label, '版本区间无效')
  }
}

function assertPlainData(value: unknown, label = '内容包清单', seen = new Set<object>()): void {
  if (value === null || ['string', 'number', 'boolean'].includes(typeof value)) return
  if (typeof value !== 'object') fail(label, '只允许 JSON 数据，不允许函数或可执行对象')
  const object = value as object
  if (seen.has(object)) fail(label, '不得包含循环引用')
  seen.add(object)
  if (!Array.isArray(value) && Object.getPrototypeOf(value) !== Object.prototype) fail(label, '只允许普通对象和数组')
  for (const [key, child] of Object.entries(value as Record<string, unknown>)) assertPlainData(child, `${label}.${key}`, seen)
  seen.delete(object)
}

function deepFreeze<T>(value: T, seen = new Set<object>()): T {
  if (value === null || typeof value !== 'object' || seen.has(value as object)) return value
  seen.add(value as object)
  for (const child of Object.values(value as Record<string, unknown>)) deepFreeze(child, seen)
  return Object.freeze(value)
}

function validateAssets(assets: Record<string, GardenAssetManifest>, label: string): number {
  if (!assets || Object.keys(assets).length === 0) fail(label, '至少需要一个资源')
  let total = 0
  for (const [id, asset] of Object.entries(assets)) {
    if (!ID_PATTERN.test(id)) fail(`${label}.${id}`, '资源 id 无效')
    if (!RELATIVE_ASSET_PATTERN.test(asset.path)) fail(`${label}.${id}`, '资源路径必须是包内 png/webp 相对路径')
    if (asset.kind !== 'image' && asset.kind !== 'sprite-sheet') fail(`${label}.${id}`, '资源类型无效')
    if (!Number.isInteger(asset.bytes) || asset.bytes <= 0) fail(`${label}.${id}`, '资源字节数无效')
    if (!Number.isInteger(asset.width) || !Number.isInteger(asset.height) || asset.width <= 0 || asset.height <= 0) fail(`${label}.${id}`, '资源尺寸无效')
    const limit = asset.kind === 'sprite-sheet' ? MAX_ANIMATION_ASSET_BYTES : MAX_IMAGE_ASSET_BYTES
    if (asset.bytes > limit) fail(`${label}.${id}`, `资源超过 ${Math.round(limit / 1024)}KB 上限`)
    total += asset.bytes
  }
  if (total > MAX_PACK_ASSET_BYTES) fail(label, `资源总量超过 ${Math.round(MAX_PACK_ASSET_BYTES / 1024 / 1024)}MB 上限`)
  return total
}

function requireAsset(assets: Record<string, GardenAssetManifest>, id: string, label: string, kind?: GardenAssetKind): GardenAssetManifest {
  nonEmpty(id, label)
  const asset = assets[id]
  if (!asset) fail(label, `引用了不存在的资源 ${id}`)
  if (kind && asset.kind !== kind) fail(label, `资源 ${id} 必须是 ${kind}`)
  return asset
}

function validateAssetList(assets: Record<string, GardenAssetManifest>, ids: string[] | undefined, label: string, exactLength?: number): void {
  if (!ids) return
  if (exactLength !== undefined && ids.length !== exactLength) fail(label, `必须恰好包含 ${exactLength} 项`)
  if (ids.length === 0) fail(label, '不能为空')
  ids.forEach((id, index) => requireAsset(assets, id, `${label}[${index}]`, 'image'))
}

function validateRect(rect: GardenRect, label: string): void {
  normalized(rect?.x, `${label}.x`)
  normalized(rect?.y, `${label}.y`)
  normalized(rect?.width, `${label}.width`)
  normalized(rect?.height, `${label}.height`)
  if (rect.width <= 0 || rect.height <= 0 || rect.x + rect.width > 1 || rect.y + rect.height > 1) fail(label, '矩形必须完整位于场景内')
}

function pointOnSegment(point: GardenPoint, start: GardenPoint, end: GardenPoint): boolean {
  const cross = (point.y - start.y) * (end.x - start.x) - (point.x - start.x) * (end.y - start.y)
  if (Math.abs(cross) > 1e-8) return false
  const dot = (point.x - start.x) * (end.x - start.x) + (point.y - start.y) * (end.y - start.y)
  const lengthSquared = (end.x - start.x) ** 2 + (end.y - start.y) ** 2
  return dot >= -1e-8 && dot <= lengthSquared + 1e-8
}

export function pointInGardenPolygon(point: GardenPoint, polygon: GardenPoint[]): boolean {
  let inside = false
  for (let index = 0, previous = polygon.length - 1; index < polygon.length; previous = index++) {
    const currentPoint = polygon[index]
    const previousPoint = polygon[previous]
    if (pointOnSegment(point, previousPoint, currentPoint)) return true
    const intersects = (currentPoint.y > point.y) !== (previousPoint.y > point.y)
      && point.x < ((previousPoint.x - currentPoint.x) * (point.y - currentPoint.y)) / (previousPoint.y - currentPoint.y) + currentPoint.x
    if (intersects) inside = !inside
  }
  return inside
}

function polygonArea(polygon: GardenPoint[]): number {
  return Math.abs(polygon.reduce((sum, point, index) => {
    const next = polygon[(index + 1) % polygon.length]
    return sum + point.x * next.y - next.x * point.y
  }, 0)) / 2
}

function isConvexPolygon(polygon: GardenPoint[]): boolean {
  let sign = 0
  for (let index = 0; index < polygon.length; index++) {
    const a = polygon[index]
    const b = polygon[(index + 1) % polygon.length]
    const c = polygon[(index + 2) % polygon.length]
    const cross = (b.x - a.x) * (c.y - b.y) - (b.y - a.y) * (c.x - b.x)
    if (Math.abs(cross) <= 1e-8) continue
    const current = Math.sign(cross)
    if (sign && current !== sign) return false
    sign = current
  }
  return sign !== 0
}

function validateBehavior(element: GardenElementManifest, label: string): void {
  const actions = element.actions ?? {}
  const behavior = element.behavior
  if (!behavior || !behavior.durations || !behavior.weights) fail(`${label}.behavior`, '必须显式声明动作时长与权重')

  for (const [action, duration] of Object.entries(behavior.durations)) {
    if (!ACTIONS.includes(action as GardenActionName) || !actions[action as GardenActionName]) fail(`${label}.behavior.durations.${action}`, '只能配置已经声明的动作')
    if (!Number.isInteger(duration.minMs) || !Number.isInteger(duration.maxMs) || duration.minMs < 250 || duration.maxMs > 120000 || duration.maxMs < duration.minMs) {
      fail(`${label}.behavior.durations.${action}`, '动作时长必须是 250–120000ms 内的有效区间')
    }
  }

  let totalWeight = 0
  for (const [action, weight] of Object.entries(behavior.weights)) {
    if (action === 'react' || !ACTIONS.includes(action as GardenActionName) || !actions[action as GardenActionName]) fail(`${label}.behavior.weights.${action}`, '只能配置已经声明的自主动作')
    finite(weight, `${label}.behavior.weights.${action}`)
    if (weight < 0 || weight > 100) fail(`${label}.behavior.weights.${action}`, '动作权重必须位于 0–100')
    totalWeight += weight
  }

  const autonomousActions = Object.keys(actions).filter(action => action !== 'react') as GardenAutonomousAction[]
  for (const action of Object.keys(actions) as GardenActionName[]) {
    if (!behavior.durations[action]) fail(`${label}.behavior.durations.${action}`, '每个动作都必须声明时长')
    if (action !== 'react' && behavior.weights[action] === undefined) fail(`${label}.behavior.weights.${action}`, '每个自主动作都必须声明权重')
    const clip = actions[action]
    if (clip && clip.format !== 'procedural' && !clip.loop) {
      const frameCount = clip.format === 'sprite-sheet' ? clip.frames : clip.assets.length
      const playbackMs = Math.ceil((frameCount / clip.fps) * 1000)
      if ((behavior.durations[action]?.minMs ?? 0) < playbackMs) fail(`${label}.behavior.durations.${action}`, `非循环动作至少需要 ${playbackMs}ms 才能播放完整`)
    }
  }
  if (autonomousActions.length > 0 && totalWeight <= 0) fail(`${label}.behavior.weights`, '至少一个自主动作的权重必须大于 0')

  const roaming = element.placement.type === 'ground-roaming' || element.placement.type === 'air-roaming'
  if (roaming) {
    if (!behavior.speed) fail(`${label}.behavior.speed`, '移动元素必须声明速度区间')
    finite(behavior.speed.min, `${label}.behavior.speed.min`)
    finite(behavior.speed.max, `${label}.behavior.speed.max`)
    if (behavior.speed.min <= 0 || behavior.speed.max > 0.5 || behavior.speed.max < behavior.speed.min) fail(`${label}.behavior.speed`, '速度必须是 0–0.5 内的有效区间')
    finite(behavior.avoidanceRadius as number, `${label}.behavior.avoidanceRadius`)
    if ((behavior.avoidanceRadius as number) < 0.01 || (behavior.avoidanceRadius as number) > 0.25) fail(`${label}.behavior.avoidanceRadius`, '避让半径必须位于 0.01–0.25')
    const movementActions = element.placement.type === 'air-roaming' ? ['fly'] : ['move', 'hop']
    if (!movementActions.some(action => actions[action as GardenActionName] && (behavior.weights[action as GardenAutonomousAction] ?? 0) > 0)) {
      fail(`${label}.behavior.weights`, '移动元素必须给对应移动动作配置正权重')
    }
  } else if (behavior.speed || behavior.avoidanceRadius !== undefined) {
    fail(`${label}.behavior`, '固定元素不能声明移动速度或避让半径')
  }
}

function validateClip(clip: GardenActionClip, action: GardenActionName, assets: Record<string, GardenAssetManifest>, label: string): void {
  if (clip.format === 'procedural') {
    if (clip.primitive !== 'sway' && clip.primitive !== 'still') fail(label, '不支持的计算动作原语')
    if (!Number.isInteger(clip.durationMs) || clip.durationMs < 500 || clip.durationMs > 60000) fail(label, '动作时长必须位于 500–60000ms')
    if (!clip.loop) fail(label, '计算动作原语必须循环')
    return
  }
  if (!Number.isInteger(clip.fps) || clip.fps < 1 || clip.fps > MAX_ANIMATION_FPS) fail(label, `帧率必须位于 1–${MAX_ANIMATION_FPS}`)
  if (clip.next && !ACTIONS.includes(clip.next)) fail(label, '后续动作无效')
  if (clip.format === 'sprite-sheet') {
    const asset = requireAsset(assets, clip.asset, `${label}.asset`, 'sprite-sheet')
    for (const [key, value] of Object.entries({ frameWidth: clip.frameWidth, frameHeight: clip.frameHeight, frames: clip.frames, columns: clip.columns, rows: clip.rows, staticFrame: clip.staticFrame })) {
      if (!Number.isInteger(value)) fail(`${label}.${key}`, '必须是整数')
    }
    if (clip.frameWidth < 1 || clip.frameHeight < 1 || clip.frameWidth > MAX_FRAME_EDGE || clip.frameHeight > MAX_FRAME_EDGE) fail(label, `单帧边长必须位于 1–${MAX_FRAME_EDGE}`)
    if (clip.frames < 1 || clip.frames > MAX_ANIMATION_FRAMES || clip.columns < 1 || clip.rows < 1 || clip.frames > clip.columns * clip.rows) fail(label, '帧数量或图集网格无效')
    if (clip.columns * clip.frameWidth > asset.width || clip.rows * clip.frameHeight > asset.height) fail(label, '声明的帧网格超出图集尺寸')
    if (clip.staticFrame < 0 || clip.staticFrame >= clip.frames) fail(label, '静态替代帧越界')
    return
  }
  validateAssetList(assets, clip.assets, `${label}.assets`)
  if (clip.assets.length > MAX_ANIMATION_FRAMES) fail(label, `帧序列不能超过 ${MAX_ANIMATION_FRAMES} 帧`)
  if (!Number.isInteger(clip.staticFrame) || clip.staticFrame < 0 || clip.staticFrame >= clip.assets.length) fail(label, '静态替代帧越界')
  void action
}

function validateScene(scene: GardenSceneManifest, assets: Record<string, GardenAssetManifest>, label: string): void {
  requireAsset(assets, scene?.dayAsset, `${label}.dayAsset`, 'image')
  requireAsset(assets, scene?.nightAsset, `${label}.nightAsset`, 'image')
  finite(scene.aspectRatio, `${label}.aspectRatio`)
  if (scene.aspectRatio < 0.5 || scene.aspectRatio > 6) fail(label, '场景宽高比必须位于 0.5–6')
  validateRect(scene.safeArea, `${label}.safeArea`)
  if (!['cover', 'contain', 'stretch'].includes(scene.fit)) fail(label, '裁切模式无效')
  if (!scene.layers?.length || new Set(scene.layers).size !== scene.layers.length) fail(label, '层级必须非空且不可重复')
  if (!scene.zones || Object.keys(scene.zones).length === 0) fail(label, '至少需要一个活动区域')
  for (const [zoneId, zone] of Object.entries(scene.zones)) {
    if (!ID_PATTERN.test(zoneId) || !['ground', 'air', 'water'].includes(zone.kind) || zone.polygon.length < 3) fail(`${label}.zones.${zoneId}`, '活动区域无效')
    zone.polygon.forEach((point, index) => {
      normalized(point.x, `${label}.zones.${zoneId}.polygon[${index}].x`)
      normalized(point.y, `${label}.zones.${zoneId}.polygon[${index}].y`)
    })
    if (polygonArea(zone.polygon) < 0.001 || !isConvexPolygon(zone.polygon)) fail(`${label}.zones.${zoneId}`, '活动区域必须是面积充足的凸多边形')
  }
  if (scene.framing !== undefined && scene.framing !== 'fit') fail(label, '未知的构图适配方式')
  if (scene.ambience !== undefined) {
    if (!Array.isArray(scene.ambience) || scene.ambience.length > 12) fail(label, '环境动效最多 12 项')
    scene.ambience.forEach((effect, index) => {
      const item = `${label}.ambience[${index}]`
      if (!['mote', 'ripple'].includes(effect.kind)) fail(item, '未知的环境动效')
      normalized(effect.x, `${item}.x`); normalized(effect.y, `${item}.y`)
      finite(effect.size, `${item}.size`); finite(effect.durationMs, `${item}.durationMs`); finite(effect.delayMs, `${item}.delayMs`)
      if (effect.size <= 0 || effect.size > 0.2 || effect.durationMs < 3000 || effect.durationMs > 60000 || effect.delayMs < 0 || effect.delayMs > 60000) fail(item, '环境动效参数超出预算')
    })
  }
  if (scene.backdrop !== undefined) {
    for (const period of ['day', 'night'] as const) {
      const tone = scene.backdrop[period]
      if (!tone) fail(`${label}.backdrop`, '必须同时声明昼夜背景参数')
      for (const field of ['brightness', 'saturation', 'contrast', 'opacity'] as const) {
        finite(tone[field], `${label}.backdrop.${period}.${field}`)
        if (tone[field] < 0 || tone[field] > 2) fail(`${label}.backdrop.${period}`, '背景参数必须位于 0–2')
      }
      if (tone.opacity > 1) fail(`${label}.backdrop.${period}.opacity`, '背景透明度必须位于 0–1')
    }
  }
  if (!scene.slots?.length) fail(label, '至少需要一个放置格位')
  scene.slots.forEach((slot, index) => {
    normalized(slot.x, `${label}.slots[${index}].x`)
    normalized(slot.y, `${label}.slots[${index}].y`)
    finite(slot.size, `${label}.slots[${index}].size`)
    if (slot.size <= 0 || slot.size > 1.5 || !scene.zones[slot.zone] || !scene.layers.includes(slot.layer)) fail(`${label}.slots[${index}]`, '格位尺寸、区域或层级无效')
    if (!pointInGardenPolygon(slot, scene.zones[slot.zone].polygon)) fail(`${label}.slots[${index}]`, '格位必须位于所属活动区域内')
  })
  ;(scene.decorations ?? []).forEach((slot, index) => {
    normalized(slot.x, `${label}.decorations[${index}].x`)
    normalized(slot.y, `${label}.decorations[${index}].y`)
    finite(slot.size, `${label}.decorations[${index}].size`)
    if (slot.size <= 0 || slot.size > 1.5 || !scene.zones[slot.zone] || !scene.layers.includes(slot.layer)) fail(`${label}.decorations[${index}]`, '格位尺寸、区域或层级无效')
    requireAsset(assets, slot.asset, `${label}.decorations[${index}].asset`, 'image')
    if (slot.anchor) {
      normalized(slot.anchor.x, `${label}.decorations[${index}].anchor.x`)
      normalized(slot.anchor.y, `${label}.decorations[${index}].anchor.y`)
    }
  })
  finite(scene.lighting?.nightBrightness, `${label}.lighting.nightBrightness`)
  finite(scene.lighting?.nightSaturation, `${label}.lighting.nightSaturation`)
  if (scene.lighting.nightBrightness < 0 || scene.lighting.nightBrightness > 2 || scene.lighting.nightSaturation < 0 || scene.lighting.nightSaturation > 2) fail(label, '夜景参数必须位于 0–2')
}

export function validateGardenPackManifest(manifest: GardenPackManifest): void {
  assertPlainData(manifest)
  if (manifest.contractVersion !== GARDEN_PACK_CONTRACT_VERSION) fail('内容包', `仅支持契约 v${GARDEN_PACK_CONTRACT_VERSION}`)
  if (!ID_PATTERN.test(manifest.id)) fail('内容包 id', '格式无效')
  if (!VERSION_PATTERN.test(manifest.version)) fail('内容包版本', '必须是语义化版本')
  nonEmpty(manifest.name, '内容包名称')
  range(manifest.runtime, '运行时版本')
  range(manifest.stateSchema, '状态版本')
  if (GARDEN_RUNTIME_VERSION < manifest.runtime.min || GARDEN_RUNTIME_VERSION > manifest.runtime.max) fail('内容包', `与当前运行时 v${GARDEN_RUNTIME_VERSION} 不兼容`)
  for (const [key, value] of Object.entries(manifest.presentation)) nonEmpty(value, `presentation.${key}`)
  if (manifest.stageLabels?.length !== 6 || manifest.stageLabels.some(label => !label.trim())) fail('stageLabels', '必须提供六个非空阶段名称')
  validateAssets(manifest.assets, 'assets')
  validateScene(manifest.scene, manifest.assets, 'scene')
  if (!manifest.elements || Object.keys(manifest.elements).length === 0) fail('elements', '至少需要一个收藏元素')
  for (const [itemId, element] of Object.entries(manifest.elements)) {
    const label = `elements.${itemId}`
    if (!ID_PATTERN.test(itemId)) fail(label, '元素 id 无效')
    nonEmpty(element.name, `${label}.name`)
    nonEmpty(element.meaning, `${label}.meaning`)
    if (element.commerce) {
      finite(element.commerce.price, `${label}.commerce.price`)
      finite(element.commerce.initialLevel, `${label}.commerce.initialLevel`)
      nonEmpty(element.commerce.category, `${label}.commerce.category`)
      if (!Number.isInteger(element.commerce.price) || element.commerce.price <= 0 || element.commerce.price > 100000) fail(`${label}.commerce.price`, '必须是 1–100000 的整数')
      if (!Number.isInteger(element.commerce.initialLevel) || element.commerce.initialLevel < 0 || element.commerce.initialLevel > 100) fail(`${label}.commerce.initialLevel`, '必须是 0–100 的整数')
    }
    if (element.stageLabels && (element.stageLabels.length !== 6 || element.stageLabels.some(stage => !stage.trim()))) fail(`${label}.stageLabels`, '必须提供六个非空阶段名称')
    validateAssetList(manifest.assets, element.stages, `${label}.stages`, 6)
    validateAssetList(manifest.assets, element.shinyStages, `${label}.shinyStages`, 6)
    validateAssetList(manifest.assets, element.matureVariants, `${label}.matureVariants`)
    validateAssetList(manifest.assets, element.shinyMatureVariants, `${label}.shinyMatureVariants`)
    normalized(element.anchor?.x, `${label}.anchor.x`)
    normalized(element.anchor?.y, `${label}.anchor.y`)
    validateRect(element.hitArea, `${label}.hitArea`)
    finite(element.defaultSize, `${label}.defaultSize`)
    if (element.defaultSize <= 0 || element.defaultSize > 2) fail(label, '默认尺寸必须位于 0–2')
    if (element.stageScales) {
      if (element.stageScales.length !== 6) fail(`${label}.stageScales`, '必须恰好包含六项')
      element.stageScales.forEach((scale, index) => {
        finite(scale, `${label}.stageScales[${index}]`)
        if (scale <= 0 || scale > 2) fail(`${label}.stageScales[${index}]`, '必须位于 0–2')
      })
    }
    if (!PLACEMENTS.includes(element.placement?.type) || !manifest.scene.zones[element.placement.zone] || !manifest.scene.layers.includes(element.placement.layer)) fail(`${label}.placement`, '放置类型、区域或层级无效')
    const zoneKind = manifest.scene.zones[element.placement.zone].kind
    if (element.placement.type === 'ground-roaming' && zoneKind !== 'ground') fail(`${label}.placement`, 'ground-roaming 必须使用 ground 区域')
    if (element.placement.type === 'air-roaming' && zoneKind !== 'air') fail(`${label}.placement`, 'air-roaming 必须使用 air 区域')
    for (const [action, clip] of Object.entries(element.actions ?? {})) {
      if (!ACTIONS.includes(action as GardenActionName)) fail(`${label}.actions.${action}`, '动作名称不受支持')
      validateClip(clip, action as GardenActionName, manifest.assets, `${label}.actions.${action}`)
      if ('next' in clip && clip.next && !element.actions[clip.next]) fail(`${label}.actions.${action}.next`, `后续动作 ${clip.next} 未声明`)
    }
    if (element.placement.type !== 'ground-roaming' && (element.actions.move || element.actions.hop)) fail(`${label}.actions`, 'move/hop 只允许 ground-roaming 元素声明')
    if (element.placement.type !== 'air-roaming' && element.actions.fly) fail(`${label}.actions`, 'fly 只允许 air-roaming 元素声明')
    validateBehavior(element, label)
    const reduced = requireAsset(manifest.assets, element.reducedMotion?.asset, `${label}.reducedMotion.asset`)
    if (element.reducedMotion.frame !== undefined) {
      const matchingClip = Object.values(element.actions).find((clip): clip is GardenSpriteSheetClip => clip?.format === 'sprite-sheet' && clip.asset === element.reducedMotion.asset)
      if (reduced.kind !== 'sprite-sheet' || !matchingClip || !Number.isInteger(element.reducedMotion.frame) || element.reducedMotion.frame < 0 || element.reducedMotion.frame >= matchingClip.frames) fail(`${label}.reducedMotion.frame`, '静态帧必须引用该元素已声明图集中的有效帧')
    }
  }
}

function hydrateAssets(assets: Record<string, GardenAssetManifest>, sources: Record<string, string>, label: string): Record<string, GardenAsset> {
  return Object.fromEntries(Object.entries(assets).map(([id, asset]) => {
    const src = sources[asset.path]
    if (!src) fail(`${label}.${id}`, `没有装载 ${asset.path}`)
    return [id, { id, ...asset, src }]
  }))
}

export function createGardenPack(manifest: GardenPackManifest, sources: Record<string, string>): GardenPack {
  validateGardenPackManifest(manifest)
  return deepFreeze({ ...manifest, assets: hydrateAssets(manifest.assets, sources, 'assets') })
}

export function validateGardenOverlayManifest(overlay: GardenOverlayManifest): void {
  assertPlainData(overlay, '覆盖包清单')
  if (overlay.contractVersion !== GARDEN_PACK_CONTRACT_VERSION) fail('覆盖包', `仅支持契约 v${GARDEN_PACK_CONTRACT_VERSION}`)
  if (!ID_PATTERN.test(overlay.id) || !VERSION_PATTERN.test(overlay.version)) fail('覆盖包', 'id 或版本无效')
  nonEmpty(overlay.name, '覆盖包名称')
  range(overlay.runtime, '覆盖包运行时版本')
  if (GARDEN_RUNTIME_VERSION < overlay.runtime.min || GARDEN_RUNTIME_VERSION > overlay.runtime.max) fail('覆盖包', `与当前运行时 v${GARDEN_RUNTIME_VERSION} 不兼容`)
  if (!overlay.targetPackIds?.length || overlay.targetPackIds.some(id => !ID_PATTERN.test(id))) fail('覆盖包', '目标内容包无效')
  validateAssets(overlay.assets, 'overlay.assets')
  if (overlay.scene?.dayAsset) requireAsset(overlay.assets, overlay.scene.dayAsset, 'overlay.scene.dayAsset', 'image')
  if (overlay.scene?.nightAsset) requireAsset(overlay.assets, overlay.scene.nightAsset, 'overlay.scene.nightAsset', 'image')
  if (overlay.scene?.lighting?.nightBrightness !== undefined) finite(overlay.scene.lighting.nightBrightness, 'overlay.scene.lighting.nightBrightness')
  if (overlay.scene?.lighting?.nightSaturation !== undefined) finite(overlay.scene.lighting.nightSaturation, 'overlay.scene.lighting.nightSaturation')
  for (const [itemId, element] of Object.entries(overlay.elements ?? {})) {
    validateAssetList(overlay.assets, element.stages, `overlay.elements.${itemId}.stages`, 6)
    validateAssetList(overlay.assets, element.shinyStages, `overlay.elements.${itemId}.shinyStages`, 6)
    validateAssetList(overlay.assets, element.matureVariants, `overlay.elements.${itemId}.matureVariants`)
    validateAssetList(overlay.assets, element.shinyMatureVariants, `overlay.elements.${itemId}.shinyMatureVariants`)
    if (element.reducedMotion) requireAsset(overlay.assets, element.reducedMotion.asset, `overlay.elements.${itemId}.reducedMotion.asset`)
  }
}

export function createGardenOverlay(manifest: GardenOverlayManifest, sources: Record<string, string>): GardenOverlay {
  validateGardenOverlayManifest(manifest)
  return deepFreeze({ ...manifest, assets: hydrateAssets(manifest.assets, sources, 'overlay.assets') })
}

export function applyGardenOverlay(pack: GardenPack, overlay: GardenOverlay): GardenPack {
  if (!overlay.targetPackIds.includes(pack.id)) fail(`覆盖包 ${overlay.id}`, `不适用于 ${pack.id}`)
  for (const id of Object.keys(overlay.assets)) if (pack.assets[id]) fail(`覆盖包 ${overlay.id}`, `资源 id ${id} 与基础包冲突`)
  for (const itemId of Object.keys(overlay.elements ?? {})) if (!pack.elements[itemId]) fail(`覆盖包 ${overlay.id}`, `引用了不存在的元素 ${itemId}`)
  const assets = { ...pack.assets, ...overlay.assets }
  const elements = Object.fromEntries(Object.entries(pack.elements).map(([itemId, element]) => [
    itemId,
    { ...element, ...(overlay.elements?.[itemId] ?? {}) },
  ]))
  const scene = {
    ...pack.scene,
    ...(overlay.scene ?? {}),
    lighting: { ...pack.scene.lighting, ...(overlay.scene?.lighting ?? {}) },
  }
  const merged: GardenPack = { ...pack, assets, elements, scene, appliedOverlays: [...(pack.appliedOverlays ?? []), overlay.id] }
  const manifest = gardenPackManifestOf(merged)
  validateGardenPackManifest(manifest)
  return deepFreeze(merged)
}

export function gardenPackManifestOf(pack: GardenPack): GardenPackManifest {
  const { appliedOverlays: _appliedOverlays, ...base } = pack
  return { ...base, assets: Object.fromEntries(Object.entries(pack.assets).map(([id, { src: _src, id: _id, ...asset }]) => [id, asset])) }
}

export function validateGardenPack(pack: GardenPack): void {
  validateGardenPackManifest(gardenPackManifestOf(pack))
  for (const [id, asset] of Object.entries(pack.assets)) nonEmpty(asset.src, `assets.${id}.src`)
}

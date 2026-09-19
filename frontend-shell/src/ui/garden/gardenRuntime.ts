import {
  MAX_ACTIVE_GARDEN_ACTORS,
  pointInGardenPolygon,
  type GardenActionClip,
  type GardenActionName,
  type GardenAutonomousAction,
  type GardenElementManifest,
  type GardenPack,
  type GardenPoint,
  type GardenSceneSlot,
} from './contentPack'

export const GARDEN_RUNTIME_FPS = 24
const MAX_STEP_MS = 100

export interface GardenActorInput {
  instanceId: string
  itemId: string
  slot: GardenSceneSlot
}

export interface GardenActorState {
  instanceId: string
  itemId: string
  placement: GardenElementManifest['placement']['type']
  zone: string
  layer: string
  homeX: number
  homeY: number
  x: number
  y: number
  target?: GardenPoint
  facing: 'left' | 'right'
  action?: GardenActionName
  actionStartedAt: number
  actionEndsAt: number
  speed: number
  randomState: number
  active: boolean
}

function hashSeed(value: string): number {
  let hash = 2166136261
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return hash >>> 0 || 0x9e3779b9
}

function nextRandom(state: number): [number, number] {
  let next = state || 0x9e3779b9
  next ^= next << 13
  next ^= next >>> 17
  next ^= next << 5
  const unsigned = next >>> 0
  return [unsigned, unsigned / 0x100000000]
}

function randomBetween(actor: GardenActorState, min: number, max: number): number {
  const [state, value] = nextRandom(actor.randomState)
  actor.randomState = state
  return min + (max - min) * value
}

function chooseAction(actor: GardenActorState, element: GardenElementManifest): GardenAutonomousAction | undefined {
  const entries = Object.entries(element.behavior.weights)
    .filter((entry): entry is [GardenAutonomousAction, number] => Number(entry[1]) > 0)
  if (!entries.length) return undefined
  const total = entries.reduce((sum, [, weight]) => sum + weight, 0)
  let cursor = randomBetween(actor, 0, total)
  for (const [action, weight] of entries) {
    cursor -= weight
    if (cursor <= 0) return action
  }
  return entries[entries.length - 1][0]
}

function randomPoint(actor: GardenActorState, polygon: GardenPoint[]): GardenPoint {
  const xs = polygon.map(point => point.x)
  const ys = polygon.map(point => point.y)
  const minX = Math.min(...xs)
  const maxX = Math.max(...xs)
  const minY = Math.min(...ys)
  const maxY = Math.max(...ys)
  for (let attempt = 0; attempt < 256; attempt++) {
    const point = {
      x: randomBetween(actor, minX, maxX),
      y: randomBetween(actor, minY, maxY),
    }
    if (pointInGardenPolygon(point, polygon)) return point
  }
  throw new Error(`活动区域无法为 ${actor.instanceId} 生成有效目标点`)
}

function isMovementAction(action: GardenActionName | undefined): boolean {
  return action === 'move' || action === 'hop' || action === 'fly'
}

function beginAction(actor: GardenActorState, element: GardenElementManifest, pack: GardenPack, action: GardenActionName | undefined, now: number): void {
  actor.action = action
  actor.actionStartedAt = now
  actor.target = undefined
  if (!action) {
    actor.actionEndsAt = Number.POSITIVE_INFINITY
    actor.speed = 0
    return
  }
  const duration = element.behavior.durations[action]
  if (!duration) throw new Error(`内容包 ${pack.id} 的 ${actor.itemId}.${action} 缺少动作时长`)
  actor.actionEndsAt = now + randomBetween(actor, duration.minMs, duration.maxMs)
  if (!isMovementAction(action)) {
    actor.speed = 0
    return
  }
  const speed = element.behavior.speed
  if (!speed) throw new Error(`内容包 ${pack.id} 的移动元素 ${actor.itemId} 缺少速度`)
  actor.speed = randomBetween(actor, speed.min, speed.max)
  actor.target = randomPoint(actor, pack.scene.zones[actor.zone].polygon)
  if (Math.abs(actor.target.x - actor.x) > 1e-6) actor.facing = actor.target.x < actor.x ? 'left' : 'right'
}

function nextAction(actor: GardenActorState, element: GardenElementManifest, pack: GardenPack, now: number): void {
  const currentClip = actor.action ? element.actions[actor.action] : undefined
  const declaredNext = currentClip && 'next' in currentClip ? currentClip.next : undefined
  beginAction(actor, element, pack, declaredNext ?? chooseAction(actor, element), now)
}

export function createGardenActors(pack: GardenPack, inputs: GardenActorInput[], now = 0): GardenActorState[] {
  return inputs.map((input, index) => {
    const element = pack.elements[input.itemId]
    if (!element) throw new Error(`内容包 ${pack.id} 未声明元素 ${input.itemId}`)
    const active = index < MAX_ACTIVE_GARDEN_ACTORS && Object.keys(element.actions).length > 0
    const actor: GardenActorState = {
      instanceId: input.instanceId,
      itemId: input.itemId,
      placement: element.placement.type,
      zone: element.placement.zone,
      layer: element.placement.layer,
      homeX: input.slot.x,
      homeY: input.slot.y,
      x: input.slot.x,
      y: input.slot.y,
      facing: 'right',
      actionStartedAt: now,
      actionEndsAt: Number.POSITIVE_INFINITY,
      speed: 0,
      randomState: hashSeed(`${pack.id}:${pack.version}:${input.instanceId}`),
      active,
    }
    if (active) beginAction(actor, element, pack, chooseAction(actor, element), now)
    return actor
  })
}

function distance(a: GardenPoint, b: GardenPoint): number {
  return Math.hypot(a.x - b.x, a.y - b.y)
}

function advanceMovement(actor: GardenActorState, others: GardenActorState[], element: GardenElementManifest, pack: GardenPack, deltaMs: number, now: number): void {
  if (!actor.target || !isMovementAction(actor.action)) return
  const toTarget = { x: actor.target.x - actor.x, y: actor.target.y - actor.y }
  const targetDistance = Math.hypot(toTarget.x, toTarget.y)
  const step = actor.speed * Math.min(deltaMs, MAX_STEP_MS) / 1000
  if (targetDistance <= step || targetDistance < 1e-5) {
    actor.x = actor.target.x
    actor.y = actor.target.y
    nextAction(actor, element, pack, now)
    return
  }

  let direction = { x: toTarget.x / targetDistance, y: toTarget.y / targetDistance }
  const radius = element.behavior.avoidanceRadius ?? 0
  for (const other of others) {
    if (other.instanceId === actor.instanceId || other.zone !== actor.zone) continue
    const separation = distance(actor, other)
    if (separation <= 1e-6 || separation >= radius) continue
    const strength = (radius - separation) / radius
    direction.x += ((actor.x - other.x) / separation) * strength
    direction.y += ((actor.y - other.y) / separation) * strength
  }
  const directionLength = Math.hypot(direction.x, direction.y) || 1
  direction = { x: direction.x / directionLength, y: direction.y / directionLength }
  let candidate = { x: actor.x + direction.x * step, y: actor.y + direction.y * step }
  const polygon = pack.scene.zones[actor.zone].polygon
  if (!pointInGardenPolygon(candidate, polygon)) {
    candidate = { x: actor.x + (toTarget.x / targetDistance) * step, y: actor.y + (toTarget.y / targetDistance) * step }
  }
  actor.x = candidate.x
  actor.y = candidate.y
  if (Math.abs(direction.x) > 1e-6) actor.facing = direction.x < 0 ? 'left' : 'right'
}

export function advanceGardenActors(current: GardenActorState[], pack: GardenPack, now: number, deltaMs: number): GardenActorState[] {
  const next = current.map(actor => ({ ...actor, target: actor.target ? { ...actor.target } : undefined }))
  for (const actor of next) {
    if (!actor.active) continue
    const element = pack.elements[actor.itemId]
    if (now >= actor.actionEndsAt) nextAction(actor, element, pack, now)
    advanceMovement(actor, next, element, pack, deltaMs, now)
  }
  return next
}

export function reactGardenActor(current: GardenActorState[], pack: GardenPack, instanceId: string, now: number): GardenActorState[] {
  return current.map(existing => {
    if (existing.instanceId !== instanceId || !existing.active) return existing
    const element = pack.elements[existing.itemId]
    if (!element.actions.react) return existing
    const actor = { ...existing, target: undefined }
    beginAction(actor, element, pack, 'react', now)
    return actor
  })
}

export function gardenClipFrame(clip: GardenActionClip | undefined, startedAt: number, now: number, reducedMotion: boolean): number {
  if (!clip || clip.format === 'procedural') return 0
  if (reducedMotion) return clip.staticFrame
  const frames = clip.format === 'sprite-sheet' ? clip.frames : clip.assets.length
  const elapsedFrames = Math.max(0, Math.floor(((now - startedAt) / 1000) * clip.fps))
  return clip.loop ? elapsedFrames % frames : Math.min(frames - 1, elapsedFrames)
}

export function gardenActorIsDynamic(actor: GardenActorState, element: GardenElementManifest): boolean {
  if (!actor.active || !actor.action) return false
  const clip = element.actions[actor.action]
  if (!clip) return false
  if (isMovementAction(actor.action) || clip.format !== 'procedural') return true
  return clip.primitive === 'sway'
}

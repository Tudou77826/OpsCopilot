import { describe, expect, it } from 'vitest'
import { createGardenPack, pointInGardenPolygon, type GardenActionName, type GardenPackManifest } from './contentPack'
import { advanceGardenActors, createGardenActors, gardenClipFrame, reactGardenActor } from './gardenRuntime'

function runtimePack(id: string, action: GardenActionName, placement: 'ambient' | 'ground-roaming' | 'air-roaming' = 'ambient') {
  const movement = action === 'move' || action === 'hop' || action === 'fly'
  const actionClip = action === 'move' || action === 'fly'
    ? { format: 'sprite-sheet' as const, asset: 'sheet', frameWidth: 64, frameHeight: 64, frames: 8, columns: 4, rows: 2, fps: 8, loop: true, staticFrame: 2, flipX: true }
    : { format: 'frame-sequence' as const, assets: ['actor', 'actor'], fps: 2, loop: true, staticFrame: 0 }
  const zone = placement === 'air-roaming' ? 'sky' : 'meadow'
  const manifest: GardenPackManifest = {
    contractVersion: 2, id, version: '1.0.0', name: id, runtime: { min: 2, max: 2 }, stateSchema: { min: 2, max: 2 },
    presentation: { title: '测试场景', unit: '个', empty: '空', maxLevel: '满级' },
    stageLabels: ['一', '二', '三', '四', '五', '六'],
    assets: {
      day: { path: 'day.webp', kind: 'image', bytes: 10, width: 100, height: 50 },
      night: { path: 'night.webp', kind: 'image', bytes: 10, width: 100, height: 50 },
      actor: { path: 'actor.webp', kind: 'image', bytes: 10, width: 64, height: 64 },
      sheet: { path: 'sheet.webp', kind: 'sprite-sheet', bytes: 10, width: 256, height: 128 },
    },
    scene: {
      dayAsset: 'day', nightAsset: 'night', aspectRatio: 2, safeArea: { x: 0, y: 0, width: 1, height: 1 }, fit: 'cover',
      zones: {
        meadow: { kind: 'ground', polygon: [{ x: 0.05, y: 0.5 }, { x: 0.95, y: 0.5 }, { x: 0.95, y: 0.95 }, { x: 0.05, y: 0.95 }] },
        sky: { kind: 'air', polygon: [{ x: 0.05, y: 0.05 }, { x: 0.95, y: 0.05 }, { x: 0.95, y: 0.48 }, { x: 0.05, y: 0.48 }] },
      },
      layers: ['near'], slots: [{ x: 0.5, y: placement === 'air-roaming' ? 0.25 : 0.75, size: 0.5, zone, layer: 'near' }], decorations: [],
      lighting: { nightBrightness: 0.7, nightSaturation: 0.8 },
    },
    elements: {
      animal: {
        name: '测试动物', meaning: '测试', stages: ['actor', 'actor', 'actor', 'actor', 'actor', 'actor'],
        anchor: { x: 0.5, y: 0.9 }, hitArea: { x: 0, y: 0, width: 1, height: 1 }, defaultSize: 1,
        placement: { type: placement, zone, layer: 'near' },
        actions: { [action]: actionClip, react: { format: 'frame-sequence', assets: ['actor', 'actor'], fps: 2, loop: false, staticFrame: 0, next: action } },
        behavior: {
          durations: { [action]: { minMs: 4000, maxMs: 4000 }, react: { minMs: 1000, maxMs: 1000 } },
          weights: { [action]: 1 },
          ...(movement ? { speed: { min: 0.1, max: 0.1 }, avoidanceRadius: 0.08 } : {}),
        },
        reducedMotion: { asset: 'actor' },
      },
    },
  }
  const sources = Object.fromEntries(Object.values(manifest.assets).map(asset => [asset.path, asset.path]))
  return createGardenPack(manifest, sources)
}

describe('garden dynamic runtime', () => {
  it('地面动物按稳定种子在区域内移动，并根据横向路径更新朝向和纵深', () => {
    const pack = runtimePack('moving-animal', 'move', 'ground-roaming')
    const slot = pack.scene.slots[0]
    let actors = createGardenActors(pack, [
      { instanceId: 'fox-a', itemId: 'animal', slot },
      { instanceId: 'fox-b', itemId: 'animal', slot: { ...slot, x: 0.52 } },
    ], 0)
    const firstTarget = actors[0].target
    expect(actors[0].action).toBe('move')
    expect(firstTarget && pointInGardenPolygon(firstTarget, pack.scene.zones.meadow.polygon)).toBe(true)

    for (let step = 1; step <= 20; step++) actors = advanceGardenActors(actors, pack, step * 100, 100)
    expect(actors[0].x).not.toBe(slot.x)
    expect(pointInGardenPolygon(actors[0], pack.scene.zones.meadow.polygon)).toBe(true)
    expect(Math.hypot(actors[0].x - actors[1].x, actors[0].y - actors[1].y)).toBeGreaterThan(0.02)
    expect(['left', 'right']).toContain(actors[0].facing)
  })

  it.each([
    ['hop', 'ground-roaming', 'meadow'],
    ['fly', 'air-roaming', 'sky'],
  ] as const)('%s 使用同一移动原语并始终留在所属区域', (action, placement, zone) => {
    const pack = runtimePack(`${action}-animal`, action, placement)
    const slot = pack.scene.slots[0]
    let actors = createGardenActors(pack, [{ instanceId: action, itemId: 'animal', slot }], 0)
    for (let step = 1; step <= 10; step++) actors = advanceGardenActors(actors, pack, step * 100, 100)
    expect(actors[0].action).toBe(action)
    expect(pointInGardenPolygon(actors[0], pack.scene.zones[zone].polygon)).toBe(true)
    expect(actors[0].x === slot.x && actors[0].y === slot.y).toBe(false)
  })

  it.each(['eat', 'sleep'] as const)('通用状态机可以进入 %s 行为而不依赖物种代码', action => {
    const pack = runtimePack(`${action}-animal`, action)
    const actor = createGardenActors(pack, [{ instanceId: action, itemId: 'animal', slot: pack.scene.slots[0] }], 0)[0]
    expect(actor.action).toBe(action)
    expect(actor.x).toBe(actor.homeX)
    expect(actor.y).toBe(actor.homeY)
  })

  it('点击回应完成后按动作声明返回自主状态', () => {
    const pack = runtimePack('reacting-animal', 'eat')
    let actors = createGardenActors(pack, [{ instanceId: 'fox', itemId: 'animal', slot: pack.scene.slots[0] }], 0)
    actors = reactGardenActor(actors, pack, 'fox', 100)
    expect(actors[0].action).toBe('react')
    actors = advanceGardenActors(actors, pack, 1200, 1100)
    expect(actors[0].action).toBe('eat')
  })

  it('精灵图与帧序列统一按帧率推进，减少动态效果时固定到静态帧', () => {
    const sprite = runtimePack('sprite-frames', 'move', 'ground-roaming').elements.animal.actions.move
    const sequence = runtimePack('sequence-frames', 'eat').elements.animal.actions.eat
    expect(gardenClipFrame(sprite, 0, 375, false)).toBe(3)
    expect(gardenClipFrame(sprite, 0, 375, true)).toBe(2)
    expect(gardenClipFrame(sequence, 0, 750, false)).toBe(1)
  })

  it('超过同屏活动上限的元素保持静态，避免无限增加运行开销', () => {
    const pack = runtimePack('actor-budget', 'eat')
    const inputs = Array.from({ length: 14 }, (_, index) => ({ instanceId: `animal-${index}`, itemId: 'animal', slot: pack.scene.slots[0] }))
    const actors = createGardenActors(pack, inputs, 0)
    expect(actors.filter(actor => actor.active)).toHaveLength(12)
    expect(actors.slice(12).every(actor => actor.action === undefined)).toBe(true)
  })
})

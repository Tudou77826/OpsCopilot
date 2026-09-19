import React from 'react'
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import GardenScene from './GardenScene'
import {
  MAX_IMAGE_ASSET_BYTES,
  applyGardenOverlay,
  createGardenOverlay,
  createGardenPack,
  registerPack,
  validateGardenPackManifest,
  type GardenOverlayManifest,
  type GardenPackManifest,
  type GardenSpecimen,
} from './pack'

function animalManifest(id = 'animal-test'): GardenPackManifest {
  return {
    contractVersion: 2, id, version: '1.0.0', name: '林间伙伴', runtime: { min: 2, max: 2 }, stateSchema: { min: 2, max: 2 },
    presentation: { title: '林地', unit: '只', empty: '完成工作，遇见第一位伙伴。', maxLevel: '相伴' },
    stageLabels: ['幼崽', '探索', '成长', '矫健', '亲近', '伙伴'],
    assets: {
      day: { path: 'assets/forest-day.webp', kind: 'image', bytes: 1000, width: 1200, height: 600 },
      night: { path: 'assets/forest-night.webp', kind: 'image', bytes: 1000, width: 1200, height: 600 },
      fox: { path: 'assets/fox.webp', kind: 'image', bytes: 500, width: 512, height: 512 },
      'fox-walk': { path: 'assets/fox-walk.webp', kind: 'sprite-sheet', bytes: 2000, width: 256, height: 128 },
    },
    scene: {
      dayAsset: 'day', nightAsset: 'night', aspectRatio: 2, fit: 'cover', safeArea: { x: 0, y: 0, width: 1, height: 1 },
      zones: { meadow: { kind: 'ground', polygon: [{ x: 0, y: 0.5 }, { x: 1, y: 0.5 }, { x: 1, y: 1 }, { x: 0, y: 1 }] } },
      layers: ['far', 'near'], slots: [{ x: 0.5, y: 0.86, size: 0.6, zone: 'meadow', layer: 'near' }], decorations: [],
      lighting: { nightBrightness: 0.65, nightSaturation: 0.8 },
    },
    elements: {
      fox: {
        name: '巡游狐', meaning: '持续探索', stages: ['fox', 'fox', 'fox', 'fox', 'fox', 'fox'],
        anchor: { x: 0.5, y: 0.86 }, hitArea: { x: 0.08, y: 0.08, width: 0.84, height: 0.84 }, defaultSize: 1,
        placement: { type: 'ground-roaming', zone: 'meadow', layer: 'near' },
        actions: {
          idle: { format: 'frame-sequence', assets: ['fox'], fps: 2, loop: true, staticFrame: 0 },
          move: { format: 'sprite-sheet', asset: 'fox-walk', frameWidth: 64, frameHeight: 64, frames: 8, columns: 4, rows: 2, fps: 10, loop: true, staticFrame: 0, flipX: true },
          eat: { format: 'frame-sequence', assets: ['fox'], fps: 2, loop: false, staticFrame: 0, next: 'idle' },
          sleep: { format: 'frame-sequence', assets: ['fox'], fps: 1, loop: true, staticFrame: 0 },
          react: { format: 'frame-sequence', assets: ['fox'], fps: 2, loop: false, staticFrame: 0, next: 'idle' },
        },
        behavior: {
          durations: {
            idle: { minMs: 3000, maxMs: 5000 }, move: { minMs: 2000, maxMs: 4000 },
            eat: { minMs: 1000, maxMs: 1500 }, sleep: { minMs: 4000, maxMs: 8000 }, react: { minMs: 500, maxMs: 500 },
          },
          weights: { idle: 0.28, move: 0.55, eat: 0.12, sleep: 0.05 },
          speed: { min: 0.04, max: 0.08 }, avoidanceRadius: 0.08,
        },
        reducedMotion: { asset: 'fox' },
      },
    },
  }
}

function sources(manifest: GardenPackManifest): Record<string, string> {
  return Object.fromEntries(Object.values(manifest.assets).map(asset => [asset.path, `loaded:${asset.path}`]))
}

function animalSpecimen(): GardenSpecimen {
  return { itemId: 'fox', instanceId: 'fox-1', level: 30, xp: 0, shiny: false, acquiredAt: '2026-09-01', lastGrewAt: '2026-09-01', placement: { placed: true, x: .5, y: .86, scale: 1, flipX: false } }
}

describe('garden content-pack contract', () => {
  it('纯数据动物包声明移动图集，仍由统一场景组件正确装载', () => {
    const manifest = animalManifest('animal-render-test')
    manifest.elements.fox.behavior.weights = { idle: 0, move: 1, eat: 0, sleep: 0 }
    const pack = createGardenPack(manifest, sources(manifest))
    registerPack(pack)
    render(<GardenScene packId={pack.id} specimens={[animalSpecimen()]} onOpen={() => {}} />)
    const target = screen.getByRole('button', { name: '巡游狐 30 级 矫健' })
    expect(target.closest('.garden-image-object')).toHaveAttribute('data-placement', 'ground-roaming')
    expect(target.closest('.garden-image-object')).toHaveAttribute('data-action', 'move')
    const sheet = target.closest('.garden-image-object')?.querySelector('.garden-sprite-sheet') as HTMLImageElement
    expect(sheet).toHaveAttribute('src', 'loaded:assets/fox-walk.webp')
    expect(sheet.style.width).toBe('400%')
    expect(sheet.style.height).toBe('200%')
    expect(pack.elements.fox.actions.move?.format).toBe('sprite-sheet')
  })

  it('拒绝缺阶段、缺资源和无法装载的资源，不静默补图', () => {
    const shortStages = animalManifest('short-stages')
    shortStages.elements.fox.stages = ['fox']
    expect(() => validateGardenPackManifest(shortStages)).toThrow(/恰好包含 6 项/)

    const missing = animalManifest('missing-asset')
    missing.elements.fox.stages[2] = 'not-found'
    expect(() => validateGardenPackManifest(missing)).toThrow(/不存在的资源/)

    const unloaded = animalManifest('unloaded-asset')
    expect(() => createGardenPack(unloaded, {})).toThrow(/没有装载/)
  })

  it('帧动画规格越界会在登记前失败', () => {
    const badGrid = animalManifest('bad-grid')
    const move = badGrid.elements.fox.actions.move
    if (move?.format !== 'sprite-sheet') throw new Error('测试夹具错误')
    move.columns = 5
    expect(() => validateGardenPackManifest(badGrid)).toThrow(/帧网格超出图集尺寸/)

    const badFps = animalManifest('bad-fps')
    const fast = badFps.elements.fox.actions.move
    if (fast?.format !== 'sprite-sheet') throw new Error('测试夹具错误')
    fast.fps = 60
    expect(() => validateGardenPackManifest(badFps)).toThrow(/帧率必须位于/)

    const wrongZone = animalManifest('wrong-zone')
    wrongZone.scene.zones.meadow.kind = 'air'
    expect(() => validateGardenPackManifest(wrongZone)).toThrow(/ground-roaming 必须使用 ground/)
  })

  it('动态行为必须完整声明，活动区域与格位必须可安全计算', () => {
    const missingBehavior = animalManifest('missing-behavior')
    delete (missingBehavior.elements.fox as Partial<typeof missingBehavior.elements.fox>).behavior
    expect(() => validateGardenPackManifest(missingBehavior)).toThrow(/必须显式声明动作时长与权重/)

    const missingDuration = animalManifest('missing-duration')
    delete missingDuration.elements.fox.behavior.durations.move
    expect(() => validateGardenPackManifest(missingDuration)).toThrow(/每个动作都必须声明时长/)

    const clippedReaction = animalManifest('clipped-reaction')
    clippedReaction.elements.fox.behavior.durations.react = { minMs: 250, maxMs: 250 }
    expect(() => validateGardenPackManifest(clippedReaction)).toThrow(/才能播放完整/)

    const concave = animalManifest('concave-zone')
    concave.scene.zones.meadow.polygon = [
      { x: 0, y: 0.5 }, { x: 1, y: 0.5 }, { x: 0.4, y: 0.7 }, { x: 1, y: 1 }, { x: 0, y: 1 },
    ]
    expect(() => validateGardenPackManifest(concave)).toThrow(/凸多边形/)

    const outsideSlot = animalManifest('outside-slot')
    outsideSlot.scene.slots[0].y = 0.25
    expect(() => validateGardenPackManifest(outsideSlot)).toThrow(/格位必须位于/)
  })

  it('资源总体积与单资源体积都有硬门禁', () => {
    const oversized = animalManifest('oversized-asset')
    oversized.assets.fox.bytes = MAX_IMAGE_ASSET_BYTES + 1
    expect(() => validateGardenPackManifest(oversized)).toThrow(/资源超过/)

    const total = animalManifest('oversized-pack')
    for (let i = 0; i < 7; i++) total.assets[`extra-${i}`] = { path: `assets/extra-${i}.webp`, kind: 'image', bytes: MAX_IMAGE_ASSET_BYTES, width: 512, height: 512 }
    expect(() => validateGardenPackManifest(total)).toThrow(/资源总量超过/)
  })

  it('清单只接受普通 JSON 数据，拒绝可执行字段', () => {
    const executable = animalManifest('executable-pack') as unknown as Record<string, unknown>
    executable.setup = () => 'run'
    expect(() => validateGardenPackManifest(executable as unknown as GardenPackManifest)).toThrow(/只允许 JSON 数据/)

    const manifest = animalManifest('forged-registration')
    const valid = createGardenPack(manifest, sources(manifest))
    const forged = { ...valid, assets: { ...valid.assets, fox: { ...valid.assets.fox, src: '' } } }
    expect(() => registerPack(forged)).toThrow(/不能为空/)
  })

  it('登记清理只移除原对象，仍拒绝同 id 内容包静默覆盖', () => {
    const firstManifest = animalManifest('disposable-pack')
    const first = createGardenPack(firstManifest, sources(firstManifest))
    const secondManifest = animalManifest('disposable-pack')
    const second = createGardenPack(secondManifest, sources(secondManifest))
    const disposeFirst = registerPack(first)
    expect(() => registerPack(second)).toThrow(/不能被静默覆盖/)
    disposeFirst()
    const disposeSecond = registerPack(second)
    disposeFirst()
    expect(() => registerPack(first)).toThrow(/不能被静默覆盖/)
    disposeSecond()
  })

  it('季节覆盖包只能覆盖呈现资源，并生成可追踪的组合包', () => {
    const manifest = animalManifest('overlay-base')
    const pack = createGardenPack(manifest, sources(manifest))
    const overlayManifest: GardenOverlayManifest = {
      contractVersion: 2, id: 'winter', version: '1.0.0', name: '冬日', targetPackIds: [pack.id], runtime: { min: 2, max: 2 },
      assets: {
        snow: { path: 'assets/snow.webp', kind: 'image', bytes: 600, width: 1200, height: 600 },
        'winter-fox': { path: 'assets/winter-fox.webp', kind: 'image', bytes: 400, width: 512, height: 512 },
      },
      scene: { dayAsset: 'snow' },
      elements: { fox: { stages: ['winter-fox', 'winter-fox', 'winter-fox', 'winter-fox', 'winter-fox', 'winter-fox'], reducedMotion: { asset: 'winter-fox' } } },
    }
    const overlay = createGardenOverlay(overlayManifest, { 'assets/snow.webp': 'snow-url', 'assets/winter-fox.webp': 'winter-fox-url' })
    const winter = applyGardenOverlay(pack, overlay)
    expect(winter.scene.dayAsset).toBe('snow')
    expect(winter.elements.fox.stages[0]).toBe('winter-fox')
    expect(winter.elements.fox.meaning).toBe('持续探索')
    expect(winter.appliedOverlays).toEqual(['winter'])
  })
})

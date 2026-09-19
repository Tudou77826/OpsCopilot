import { describe, expect, it } from 'vitest'
import { woodlandCourtyardPack as pack } from './index'
import { placePackContent } from '../../ContentGardenScene'
import { validateGardenPackManifest, type GardenPackManifest, type GardenSpecimen } from '../../pack'
import manifest from './manifest.json'

const specimens: GardenSpecimen[] = Object.entries(pack.elements).map(([itemId, element], i) => ({
  itemId, instanceId: `spec-${i}`,
  level: 50, xp: 0, shiny: false, acquiredAt: '2026-09-01', lastGrewAt: '2026-09-01',
  placement: { placed: true, x: .12 + i * .14, y: .72 + (i % 2) * .12, scale: 1, flipX: false },
}))

describe('courtyard production composition', () => {
  it('six species have six distinct growth images and rare images at the 64px contract', () => {
    expect(specimens).toHaveLength(6)
    for (const element of Object.values(pack.elements)) {
      expect(new Set(element.stages).size).toBe(6)
      expect(new Set(element.shinyStages).size).toBe(6)
      expect(element.stageScales).toBeUndefined() // Growth is drawn into each frame, not scaled twice.
      for (const id of [...element.stages, ...element.shinyStages!]) {
        expect(pack.assets[id]).toMatchObject({ width: 64, height: 64 })
      }
    }
  })
  it('all six retain free placements without paging and survive snapshot reordering', () => {
    const first = placePackContent(pack, specimens)
    expect(first.unavailable).toBe(0)
    expect(first.placed.every(item => item.page === 0)).toBe(true)
    expect(new Set(first.placed.map(item => item.slot.zone))).toEqual(new Set(['ground']))
    expect(first.placed.map(item => item.slot.x)).toEqual(specimens.map(item => item.placement.x))
    expect(placePackContent(pack, [...specimens].reverse())).toEqual(first)
  })
  it('inventory specimens do not enter the scene or relocate existing ones', () => {
    const extra = { ...specimens[0], instanceId: 'later', acquiredAt: '2026-09-02', placement: { ...specimens[0].placement, placed: false } }
    const result = placePackContent(pack, [extra, ...specimens])
    expect(result.placed.find(item => item.spec.instanceId === 'later')).toBeUndefined()
    expect(result.placed.filter(item => item.page === 0)).toHaveLength(6)
  })
  it('backgrounds retain the scene ratio and effects have a bounded budget', () => {
    expect(pack.scene.framing).toBe('fit')
    for (const id of [pack.scene.dayAsset, pack.scene.nightAsset]) {
      expect(pack.assets[id].width / pack.assets[id].height).toBe(pack.scene.aspectRatio)
    }
    expect(pack.scene.ambience!.length).toBeLessThanOrEqual(12)
    expect(pack.scene.backdrop?.day.saturation).toBeLessThan(0.8)
    expect(pack.scene.backdrop?.night.brightness).toBeLessThan(0.9)
  })
  it('invalid ambient positions, budgets and effect kinds are rejected', () => {
    for (const effect of [
      { ...manifest.scene.ambience[0], x: 2 },
      { ...manifest.scene.ambience[0], kind: 'species-specific-magic' },
      { ...manifest.scene.ambience[0], durationMs: 1 },
    ]) {
      const bad = structuredClone(manifest)
      bad.scene.ambience = [effect]
      expect(() => validateGardenPackManifest(bad as unknown as GardenPackManifest)).toThrow()
    }
    const bad = structuredClone(manifest)
    bad.scene.ambience = Array(13).fill(manifest.scene.ambience[0])
    expect(() => validateGardenPackManifest(bad as unknown as GardenPackManifest)).toThrow()
  })
  it('rejects invalid background tone instead of silently rendering it', () => {
    const bad = structuredClone(manifest)
    bad.scene.backdrop.day.opacity = 1.2
    expect(() => validateGardenPackManifest(bad as unknown as GardenPackManifest)).toThrow(/背景透明度/)
  })
})

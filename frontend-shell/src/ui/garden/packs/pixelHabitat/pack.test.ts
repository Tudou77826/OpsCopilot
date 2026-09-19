import { describe, expect, it } from 'vitest'
import { resolveSpecimenVisual, stageLabel } from '../../pack'
import { pixelHabitatPack } from './index'

describe('pixel habitat production content pack', () => {
  it('覆盖六个成长通道并遵守像素素材规格', () => {
    expect(pixelHabitatPack.art).toEqual({ style: 'pixel', baseGrid: 32, subjectSize: 64 })
    expect(Object.keys(pixelHabitatPack.elements)).toEqual([
      'cmd-mint', 'transfer-fern', 'guard-orchid', 'knowledge-tree', 'session-tree', 'script-vine',
    ])
    for (const element of Object.values(pixelHabitatPack.elements)) {
      expect(element.stages).toHaveLength(6)
      expect(new Set(element.stages).size).toBe(6)
      expect(element.shinyStages).toHaveLength(6)
      for (const assetId of [...element.stages, ...(element.shinyStages ?? [])]) {
        expect(pixelHabitatPack.assets[assetId]).toMatchObject({ width: 64, height: 64, kind: 'image' })
      }
    }
  })

  it('每件收藏声明成熟变体池，普通与闪光各四个', () => {
    for (const element of Object.values(pixelHabitatPack.elements)) {
      // 九宫格第 6–9 格：第 6 格同时是阶段 6，另三格是成熟变体
      expect(element.matureVariants).toHaveLength(4)
      expect(element.matureVariants?.[0]).toBe(element.stages[5])
      expect(element.shinyMatureVariants).toHaveLength(4)
      for (const assetId of [...(element.matureVariants ?? []), ...(element.shinyMatureVariants ?? [])]) {
        expect(pixelHabitatPack.assets[assetId]).toMatchObject({ width: 64, height: 64, kind: 'image' })
      }
    }
  })

  it('每个元素都声明六段生长幅度，且幼苗明显小于成熟', () => {
    for (const [id, element] of Object.entries(pixelHabitatPack.elements)) {
      expect(element.stageScales, `${id} 缺少 stageScales`).toHaveLength(6)
      const scales = element.stageScales ?? []
      // 必须单调递增：生长要在显示尺寸上看得出来，而不只是画得大一点
      for (let i = 1; i < scales.length; i++) expect(scales[i]).toBeGreaterThan(scales[i - 1])
      expect(scales[0]).toBeLessThan(0.6)
      expect(scales[5]).toBe(1)
    }
  })

  it('每个区域都有足够格位，六件收藏同屏不翻页', () => {
    const byZone = new Map<string, number>()
    for (const element of Object.values(pixelHabitatPack.elements)) {
      const zone = element.placement.zone
      byZone.set(zone, (byZone.get(zone) ?? 0) + 1)
    }
    for (const [zone, used] of byZone) {
      const slots = pixelHabitatPack.scene.slots.filter(slot => slot.zone === zone).length
      // 格位少于元素数就会翻页；默认收藏目录是六件，必须一屏装下
      expect(slots, `区域 ${zone} 的格位少于元素数`).toBeGreaterThanOrEqual(used)
    }
  })

  it('物种之间保持真实的体量关系：乔木最大、菌类最小', () => {
    const size = (id: string) => pixelHabitatPack.elements[id].defaultSize
    expect(size('knowledge-tree')).toBeGreaterThan(size('script-vine'))
    expect(size('script-vine')).toBeGreaterThan(size('transfer-fern'))
    expect(size('transfer-fern')).toBeGreaterThan(size('cmd-mint'))
    expect(size('cmd-mint')).toBeGreaterThan(size('session-tree'))
    // 成熟蘑菇不能和成熟乔木一样大
    expect(size('knowledge-tree') / size('session-tree')).toBeGreaterThan(2)
  })

  it('同一成熟阶段按个体稳定选用不同变体', () => {
    const picks = new Set(
      ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'].map(suffix =>
        resolveSpecimenVisual(pixelHabitatPack, 'guard-orchid', 60, false, `orchid-${suffix}`)?.assetId,
      ),
    )
    expect(picks.size).toBeGreaterThan(1)
  })

  it('六个槽位都是原地生长，动作完全由清单声明', () => {
    for (const element of Object.values(pixelHabitatPack.elements)) {
      expect(element.placement.type).toBe('ambient')
      expect(Object.keys(element.actions)).toEqual(['idle'])
      expect(element.actions.idle).toMatchObject({ format: 'procedural', primitive: 'sway' })
    }
    expect(pixelHabitatPack.scene.zones.meadow.kind).toBe('ground')
  })

  it('不同题材可以提供自己的成长阶段文案与尺寸变化', () => {
    expect(stageLabel(pixelHabitatPack, 0, 'cmd-mint')).toBe('嫩芽')
    expect(stageLabel(pixelHabitatPack, 0, 'session-tree')).toBe('孢子')
    expect(stageLabel(pixelHabitatPack, 5, 'script-vine')).toBe('成廊')
    const young = resolveSpecimenVisual(pixelHabitatPack, 'session-tree', 0, false, 'fungus-a')
    const mature = resolveSpecimenVisual(pixelHabitatPack, 'session-tree', 50, false, 'fungus-a')
    expect(young?.defaultSize).toBeLessThan(mature?.defaultSize ?? 0)
  })

  it('普通与闪光品质解析为不同像素资源', () => {
    for (const itemId of Object.keys(pixelHabitatPack.elements)) {
      const normal = resolveSpecimenVisual(pixelHabitatPack, itemId, 50, false, `${itemId}-1`)
      const shiny = resolveSpecimenVisual(pixelHabitatPack, itemId, 50, true, `${itemId}-1`)
      expect(shiny?.assetId).not.toBe(normal?.assetId)
      expect(shiny?.src).not.toBe(normal?.src)
    }
  })
})

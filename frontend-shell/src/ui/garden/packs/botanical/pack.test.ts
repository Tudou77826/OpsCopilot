import { describe, expect, it } from 'vitest'
import { botanicalPack } from './index'
import { NUM_STAGES, resolveSpecimenVisual, stageLabel } from '../../pack'

describe('botanical storybook content pack', () => {
  it('清单、场景与元素均通过通用内容包契约装载', () => {
    expect(botanicalPack.contractVersion).toBe(2)
    expect(botanicalPack.id).toBe('botanical-storybook')
    expect(botanicalPack.stageLabels).toHaveLength(NUM_STAGES)
    expect(botanicalPack.scene.zones.meadow.kind).toBe('ground')
    expect(botanicalPack.scene.zones.sky.kind).toBe('air')
    expect(Object.keys(botanicalPack.elements)).toEqual(['cmd-mint', 'transfer-fern', 'guard-orchid'])
    for (const element of Object.values(botanicalPack.elements)) {
      expect(element.stages).toHaveLength(NUM_STAGES)
      expect(element.reducedMotion.asset).toBeTruthy()
    }
  })

  it('按阶段与品质解析资源，锚点统一为归一化坐标', () => {
    const mint = resolveSpecimenVisual(botanicalPack, 'cmd-mint', 12, false, 'i_1')
    expect(mint?.src).toBeTruthy()
    expect(mint?.anchor).toEqual({ x: 0.5, y: 0.9 })

    const shiny = resolveSpecimenVisual(botanicalPack, 'guard-orchid', 50, true, 'i_1')
    const normal = resolveSpecimenVisual(botanicalPack, 'guard-orchid', 50, false, 'i_1')
    expect(shiny?.assetId).toBe('orchid-shiny')
    expect(shiny?.src).not.toBe(normal?.src)
    expect(stageLabel(botanicalPack, 5)).toBe('成熟')
  })

  it('未收录元素明确不可用，不跨包回退为植物 SVG', () => {
    expect(resolveSpecimenVisual(botanicalPack, 'session-tree', 22, false, 'i_1')).toBeUndefined()
    expect(resolveSpecimenVisual(undefined, 'guard-orchid', 30, false, 'i_1')).toBeUndefined()
  })

  it('成熟姿态按 instanceId 确定选择，闪光与普通变体彼此隔离', () => {
    const withVariants = {
      ...botanicalPack,
      elements: {
        ...botanicalPack.elements,
        'guard-orchid': {
          ...botanicalPack.elements['guard-orchid'],
          matureVariants: ['mint', 'fern', 'orchid'],
          shinyMatureVariants: ['orchid-shiny'],
        },
      },
    }
    const first = resolveSpecimenVisual(withVariants, 'guard-orchid', 50, false, 'i_a')
    const again = resolveSpecimenVisual(withVariants, 'guard-orchid', 50, false, 'i_a')
    expect(first?.assetId).toBe(again?.assetId)
    expect(resolveSpecimenVisual(withVariants, 'guard-orchid', 12, false, 'i_a')?.assetId).toBe('orchid')
    expect(resolveSpecimenVisual(withVariants, 'guard-orchid', 50, true, 'i_a')?.assetId).toBe('orchid-shiny')
  })
})

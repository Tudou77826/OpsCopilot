import { describe, expect, it } from 'vitest'
import { botanicalPack } from './index'
import { NUM_STAGES, resolveSpecimenImage, stageLabel, type GardenPack, type GardenPackSpecies } from '../../pack'

/**
 * image 画法包的契约：清单形状、阶段图数量、解析函数的确定性。
 * 题材无关性由形状保证——任何题材（动物/建筑）的包都长这个样子。
 */
describe('botanical-image pack', () => {
  it('六个语义位都有名称与含义，阶段词表与状态层阶段数等长', () => {
    const entries: GardenPackSpecies[] = Object.values(botanicalPack.species)
    expect(entries).toHaveLength(6)
    for (const entry of entries) {
      expect(entry.name.trim()).not.toBe('')
      expect(entry.meaning.trim()).not.toBe('')
    }
    expect(botanicalPack.stageLabels).toHaveLength(NUM_STAGES)
    expect(stageLabel(botanicalPack, 0)).toBe('发芽')
    expect(stageLabel(botanicalPack, 5)).toBe('成熟')
    // 缺省词表兜底
    expect(stageLabel(undefined, 2)).toBe('分枝')
  })

  it('resolveSpecimenImage：有图物种按阶段取图，闪光优先替代图，锚点缺省 50%/88%', () => {
    const image = resolveSpecimenImage(botanicalPack, 'cmd-mint', 12, false, 'i_1')
    expect(image?.src).toBeTruthy()
    expect(image?.anchor).toEqual({ x: 50, y: 90 })

    const shiny = resolveSpecimenImage(botanicalPack, 'guard-orchid', 30, true, 'i_1')
    expect(shiny?.src).not.toBe(resolveSpecimenImage(botanicalPack, 'guard-orchid', 30, false, 'i_1')?.src)
  })

  it('未覆盖的物种返回 undefined（场景层回退内置 SVG 画法）', () => {
    expect(resolveSpecimenImage(botanicalPack, 'session-tree', 22, false, 'i_1')).toBeUndefined()
    expect(resolveSpecimenImage(undefined, 'guard-orchid', 30, false, 'i_1')).toBeUndefined()
  })

  it('成熟姿态按 instanceId 确定性挑选：同一株永不变脸，不同株可能不同', () => {
    const withVariants: GardenPack = {
      ...botanicalPack,
      images: {
        ...botanicalPack.images!,
        species: {
          ...botanicalPack.images!.species,
          'guard-orchid': {
            ...botanicalPack.images!.species['guard-orchid'],
            matureVariants: ['a.webp', 'b.webp', 'c.webp'],
          },
        },
      },
    }
    const first = resolveSpecimenImage(withVariants, 'guard-orchid', 30, false, 'i_a')
    const again = resolveSpecimenImage(withVariants, 'guard-orchid', 30, false, 'i_a')
    expect(first?.src).toBe(again?.src)
    // 非最高阶段不吃姿态变体
    const young = resolveSpecimenImage(withVariants, 'guard-orchid', 12, false, 'i_a')
    expect(young?.src).toBe(botanicalPack.images!.species['guard-orchid'].stages[2])
  })
})

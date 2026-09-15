import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import GardenPanel from './GardenPanel'
import GardenScene from './GardenScene'
import ImageSpecimen from './packs/imageSpecimen'
import { registerPack, type GardenPack, type GardenSpecimen } from './pack'

const building: GardenPack = {
  id: 'architecture-test', name: '城市', art: 'image', schemaVersions: '1',
  presentation: { title: '城市', unit: '座', empty: '完成任务，建造第一座建筑。', maxLevel: '落成' },
  species: { house: { name: '图书馆', meaning: '知识的积累' } },
  stageLabels: ['规划', '地基', '框架', '外墙', '装饰', '落成'],
  images: { scene: { day: 'city-day.webp', night: 'city-night.webp' }, layout: { aspectRatio: 2, slots: [{ x: 45, y: 85, size: .65 }], nightBrightness: .7, nightSaturation: .9, sway: false }, species: { house: { stages: ['1.webp', '2.webp', '3.webp', '4.webp', '5.webp', '6.webp'], anchor: { x: 35, y: 92 } } } },
}
registerPack(building)
const specimen = (id: string): GardenSpecimen => ({ speciesId: 'house', instanceId: id, level: 30, xp: 0, shiny: false, acquiredAt: '2026-09-01', lastGrewAt: '2026-09-01' })

describe('图片题材契约', () => {
  it('建筑包通过同一个面板展示题材文案，既不摇晃也不渲染植物SVG', async () => {
    const { container } = render(<GardenPanel packId={building.id} host={{ snapshot: async () => ({ schemaVersion: 1, ruleVersion: 1, gardenLevel: 2, specimens: [specimen('a')], pitySinceShiny: 0, pending: [] }), dismiss: async () => {} }} />)
    const target = await screen.findByRole('button', { name: '图书馆 30 级 落成' })
    expect(screen.getByText(/等级 2 · 1 座/)).toBeTruthy()
    expect(target.getAttribute('data-sway')).toBe('false')
    expect(container.querySelector('svg')).toBeNull()
    fireEvent.click(target)
    expect(screen.getByRole('dialog', { name: '图书馆 收藏详情' })).toBeTruthy()
    expect(screen.queryByText(/常青/)).toBeNull()
  })
  it('收藏超过格位时可翻页访问，不丢弃数据', () => {
    const onOpen = vi.fn()
    render(<GardenScene packId={building.id} specimens={[specimen('a'), specimen('b')]} pending={[]} onOpen={onOpen} onDismissPending={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: '下一组收藏' }))
    fireEvent.click(screen.getByRole('button', { name: '图书馆 30 级 落成' }))
    expect(onOpen).toHaveBeenCalledWith('b')
    expect(screen.getByRole('button', { name: '下一组收藏' })).toBeDisabled()
  })
  it('资源缺失显式报告，不切换到植物布景', () => {
    const { container } = render(<GardenScene packId={building.id} specimens={[]} pending={[]} onOpen={() => {}} onDismissPending={() => {}} />)
    fireEvent.error(container.querySelector('.garden-image-background')!)
    expect(screen.getByRole('alert').textContent).toMatch(/加载失败/)
    expect(container.querySelector('svg')).toBeNull()
  })
  it('水平垂直锚点同时参与定位，非方形图片保持自身比例', () => {
    render(<ImageSpecimen src="wide-building.webp" anchor={{ x: 35, y: 92 }} size={160} label="图书馆" />)
    const image = screen.getByRole('img', { name: '图书馆' })
    expect(image.style.transform).toBe('translate(-35%, -92%)')
    expect(image.style.height).toBe('auto')
  })
  it('不完整场景和非法锚点在注册时被拒绝', () => {
    expect(() => registerPack({ ...building, images: { ...building.images!, layout: undefined } })).toThrow(/布局/)
    expect(() => registerPack({ ...building, images: { ...building.images!, species: { house: { stages: Array(6).fill('x'), anchor: { x: 180, y: 88 } } } } })).toThrow(/锚点/)
  })
  it('单个收藏图片加载失败保留明确反馈，换资源后可以重新加载', () => {
    const { rerender } = render(<ImageSpecimen src="broken.webp" anchor={{ x: 50, y: 90 }} size={160} label="图书馆" />)
    fireEvent.error(screen.getByRole('img'))
    expect(screen.getByRole('status').textContent).toContain('素材加载失败')
    rerender(<ImageSpecimen src="fixed.webp" anchor={{ x: 50, y: 90 }} size={160} label="图书馆" />)
    expect(screen.getByRole('img').getAttribute('src')).toBe('fixed.webp')
  })
})

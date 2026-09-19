import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import GardenPanel from './GardenPanel'
import GardenScene from './GardenScene'
import ImageSpecimen from './packs/imageSpecimen'
import ContentGardenScene from './ContentGardenScene'
import { createGardenPack, registerPack, type GardenPackManifest, type GardenSpecimen } from './pack'

const buildingManifest: GardenPackManifest = {
  contractVersion: 2,
  id: 'architecture-test', version: '1.0.0', name: '城市', runtime: { min: 2, max: 2 }, stateSchema: { min: 2, max: 2 },
  presentation: { title: '城市', unit: '座', empty: '完成任务，建造第一座建筑。', maxLevel: '落成' },
  stageLabels: ['规划', '地基', '框架', '外墙', '装饰', '落成'],
  assets: {
    day: { path: 'assets/city-day.webp', kind: 'image', bytes: 1000, width: 1200, height: 600 },
    night: { path: 'assets/city-night.webp', kind: 'image', bytes: 1000, width: 1200, height: 600 },
    house: { path: 'assets/house.webp', kind: 'image', bytes: 500, width: 512, height: 512 },
  },
  scene: {
    dayAsset: 'day', nightAsset: 'night', aspectRatio: 2, fit: 'stretch',
    safeArea: { x: 0, y: 0, width: 1, height: 1 },
    zones: { city: { kind: 'ground', polygon: [{ x: 0, y: 0.5 }, { x: 1, y: 0.5 }, { x: 1, y: 1 }, { x: 0, y: 1 }] } },
    layers: ['back', 'front'], slots: [{ x: 0.45, y: 0.85, size: 0.65, zone: 'city', layer: 'front' }], decorations: [],
    lighting: { nightBrightness: 0.7, nightSaturation: 0.9 },
  },
  elements: {
    house: {
      name: '图书馆', meaning: '知识的积累',
      stages: ['house', 'house', 'house', 'house', 'house', 'house'],
      anchor: { x: 0.35, y: 0.92 }, hitArea: { x: 0.05, y: 0.05, width: 0.9, height: 0.95 }, defaultSize: 1,
      placement: { type: 'anchored', zone: 'city', layer: 'front' }, actions: {},
      behavior: { durations: {}, weights: {} }, reducedMotion: { asset: 'house' },
    },
  },
}
const sources = { 'assets/city-day.webp': 'city-day.webp', 'assets/city-night.webp': 'city-night.webp', 'assets/house.webp': 'house.webp' }
const building = createGardenPack(buildingManifest, sources)
registerPack(building)
const specimen = (id: string, x = .45): GardenSpecimen => ({
  itemId: 'house', instanceId: id, level: 50, xp: 0, shiny: false,
  acquiredAt: '2026-09-01', lastGrewAt: '2026-09-01',
  placement: { placed: true, x, y: .85, scale: 1, flipX: false },
})

const snapshot = (specimens: GardenSpecimen[]) => ({
  schemaVersion: 2, ruleVersion: 2, gardenLevel: specimens.length, balance: 120, earned: 0, spent: 0, specimens, changeSignal: null,
})
const host = (specimens: GardenSpecimen[]) => ({
  snapshot: async () => snapshot(specimens), signal: async () => null,
  purchase: async () => { throw new Error('测试未实现购买') },
  place: async () => snapshot(specimens), stow: async () => snapshot(specimens),
})

describe('通用内容场景', () => {
  it('完整构图面板高度跟随实际容器宽度，而非宿主传入的固定高度', async () => {
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    const fitted = { ...building, id: 'responsive-building', scene: { ...building.scene, framing: 'fit' as const } }
    const unregister = registerPack(fitted)
    try {
      const { container, unmount } = render(<GardenPanel packId={fitted.id} height={320} host={host([])} />)
      await screen.findByText(/等级 0 · 0 座/)
      const panel = container.querySelector<HTMLElement>('.garden-picture-panel')!
      expect(panel.parentElement?.className).toBe('garden-panel-frame')
      expect(panel.style.height).toContain('100cqw')
      expect(panel.style.height).not.toBe('320px')
      unmount()
    } finally {
      unregister()
      vi.unstubAllGlobals()
    }
  })
  it('完整构图按容器等比缩放，卸载释放尺寸观察器', () => {
    const disconnect = vi.fn()
    const observe = vi.fn()
    vi.stubGlobal('ResizeObserver', class { observe = observe; disconnect = disconnect })
    const width = vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(600)
    const height = vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(200)
    try {
      const fitted = { ...building, scene: { ...building.scene, framing: 'fit' as const } }
      const { container, unmount } = render(<ContentGardenScene pack={fitted} specimens={[]} onOpen={() => {}} />)
      const world = container.querySelector<HTMLElement>('.garden-image-world')!
      expect(world.style.width).toBe('400px')
      expect(world.style.height).toBe('200px')
      expect(observe).toHaveBeenCalledOnce()
      unmount()
      expect(disconnect).toHaveBeenCalledOnce()
    } finally {
      width.mockRestore()
      height.mockRestore()
      vi.unstubAllGlobals()
    }
  })
  it('建筑包通过同一个面板展示，运行时只暴露声明式放置类型', async () => {
    const { container } = render(<GardenPanel packId={building.id} host={host([specimen('a')])} />)
    const target = await screen.findByRole('button', { name: '图书馆 50 级 落成' })
    expect(screen.getByText(/等级 1 · 1 座/)).toBeTruthy()
    expect(target.closest('.garden-image-object')?.getAttribute('data-placement')).toBe('anchored')
    expect(container.querySelector('svg')).toBeNull()
    fireEvent.click(target)
    expect(screen.getByRole('dialog', { name: '图书馆 收藏详情' })).toBeTruthy()
    expect(screen.queryByText(/常青/)).toBeNull()
  })

  it('多个收藏使用各自自由坐标，同屏展示且不依赖格位分页', () => {
    const onOpen = vi.fn()
    const { container } = render(<GardenScene packId={building.id} specimens={[specimen('a', .3), specimen('b', .7)]} onOpen={onOpen} />)
    const targets = screen.getAllByRole('button', { name: '图书馆 50 级 落成' })
    expect(targets).toHaveLength(2)
    expect(container.querySelectorAll('.garden-image-object')[0]).toHaveStyle({ left: '30%' })
    expect(container.querySelectorAll('.garden-image-object')[1]).toHaveStyle({ left: '70%' })
    fireEvent.click(targets[1])
    expect(onOpen).toHaveBeenCalledWith('b')
  })

  it('背景资源缺失显式报告，不切换到其他题材', () => {
    const { container } = render(<GardenScene packId={building.id} specimens={[]} onOpen={() => {}} />)
    fireEvent.error(container.querySelector('.garden-image-background')!)
    expect(screen.getByRole('alert').textContent).toMatch(/素材加载失败/)
    expect(container.querySelector('svg')).toBeNull()
  })

  it('归一化锚点参与定位，非方形图片保持自身比例', () => {
    render(<ImageSpecimen src="wide-building.webp" anchor={{ x: 0.35, y: 0.92 }} size={160} label="图书馆" />)
    const image = screen.getByRole('img', { name: '图书馆' })
    expect(image.style.transform).toBe('translate(-35%, -92%)')
    expect(image.style.height).toBe('auto')
  })

  it('未登记内容包明确失败，不回退到默认包', () => {
    render(<GardenScene packId="missing-pack" specimens={[specimen('a')]} onOpen={() => {}} />)
    expect(screen.getByRole('alert').textContent).toContain('missing-pack 未登记')
    expect(screen.queryByRole('button', { name: /图书馆/ })).toBeNull()
  })

  it('单个收藏图片加载失败保留明确反馈，换资源后可以重新加载', () => {
    const { rerender } = render(<ImageSpecimen src="broken.webp" anchor={{ x: 0.5, y: 0.9 }} size={160} label="图书馆" />)
    fireEvent.error(screen.getByRole('img'))
    expect(screen.getByRole('status').textContent).toContain('素材加载失败')
    rerender(<ImageSpecimen src="fixed.webp" anchor={{ x: 0.5, y: 0.9 }} size={160} label="图书馆" />)
    expect(screen.getByRole('img').getAttribute('src')).toBe('fixed.webp')
  })
})

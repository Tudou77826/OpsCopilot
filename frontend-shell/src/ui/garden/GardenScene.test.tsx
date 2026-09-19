import React from 'react'
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import GardenScene from './GardenScene'
import GardenPanel from './GardenPanel'
import { registerPack, type GardenSpecimen, type GardenSnapshot } from './pack'
import { createGardenPack, type GardenPackManifest } from './contentPack'

// 自带测试包：这些用例检验的是场景与面板对内容包的渲染行为，
// 与当前上线的是哪个主题无关，所以不依赖任何生产内容包。
// 生产内容包自身的完备性（元素覆盖、阶段文案、资源规格）由内容包目录内的用例负责。
// id 用独立的 scene-probe：生产内容包现在会登记 ops-pixel-habitat，测试包不能占同一个 id，
// 因此下面每个渲染都显式传 packId。
const manifest: GardenPackManifest = {
  contractVersion: 2, id: 'scene-probe', version: '1.0.0', name: '场景探针', runtime: { min: 2, max: 2 }, stateSchema: { min: 2, max: 2 },
  presentation: { title: '测试场景', unit: '位伙伴', empty: '空测试场景，等第一位伙伴。', maxLevel: '满级' },
  stageLabels: ['阶段一', '阶段二', '阶段三', '阶段四', '阶段五', '阶段六'],
  assets: {
    day: { path: 'day.webp', kind: 'image', bytes: 10, width: 100, height: 50 },
    night: { path: 'night.webp', kind: 'image', bytes: 10, width: 100, height: 50 },
    actor: { path: 'actor.webp', kind: 'image', bytes: 10, width: 64, height: 64 },
  },
  scene: {
    dayAsset: 'day', nightAsset: 'night', aspectRatio: 2, safeArea: { x: 0, y: 0, width: 1, height: 1 }, fit: 'cover',
    zones: {
      meadow: { kind: 'ground', polygon: [{ x: 0.05, y: 0.5 }, { x: 0.95, y: 0.5 }, { x: 0.95, y: 0.95 }, { x: 0.05, y: 0.95 }] },
    },
    layers: ['near'], slots: [{ x: 0.5, y: 0.75, size: 0.5, zone: 'meadow', layer: 'near' }], decorations: [],
    lighting: { nightBrightness: 0.7, nightSaturation: 0.8 },
  },
  elements: {
    'cmd-mint': {
      name: '测试薄荷', meaning: '测试', stages: ['actor', 'actor', 'actor', 'actor', 'actor', 'actor'],
      shinyStages: ['actor', 'actor', 'actor', 'actor', 'actor', 'actor'],
      anchor: { x: 0.5, y: 0.9 }, hitArea: { x: 0, y: 0, width: 1, height: 1 }, defaultSize: 1,
      commerce: { price: 35, category: '香草', initialLevel: 50 },
      placement: { type: 'ambient', zone: 'meadow', layer: 'near' },
      actions: { idle: { format: 'procedural', primitive: 'sway', durationMs: 9600, loop: true } },
      behavior: { durations: { idle: { minMs: 4000, maxMs: 4000 } }, weights: { idle: 1 } },
      reducedMotion: { asset: 'actor' },
    },
  },
}
const pack = createGardenPack(manifest, Object.fromEntries(Object.values(manifest.assets).map(asset => [asset.path, asset.path])))
let dispose: () => void

beforeAll(() => { dispose = registerPack(pack) })
afterAll(() => { dispose() })

function spec(overrides: Partial<GardenSpecimen>): GardenSpecimen {
  return {
    itemId: 'cmd-mint', instanceId: 'i_1', level: 0, xp: 0, shiny: false,
    acquiredAt: '2026-09-01T00:00:00Z', lastGrewAt: '2026-09-01T00:00:00Z',
    placement: { placed: true, x: .5, y: .75, scale: 1, flipX: false }, ...overrides,
  }
}

const host = (snapshot: GardenSnapshot) => ({
  snapshot: () => Promise.resolve(snapshot), signal: () => Promise.resolve(null),
  purchase: () => Promise.reject(new Error('测试未实现购买')),
  place: () => Promise.resolve(snapshot), stow: () => Promise.resolve(snapshot),
})

describe('GardenScene', () => {
  it('按内容包渲染收藏，未收录的条目只在页脚计数、不进入场景', () => {
    const specimens = [
      spec({ instanceId: 'unknown', itemId: 'session-tree', level: 22 }),
      spec({ instanceId: 'shiny', level: 50, shiny: true }),
    ]
    const onOpen = vi.fn()
    const { container } = render(<GardenScene specimens={specimens} packId="scene-probe" onOpen={onOpen} />)
    const shiny = screen.getByRole('button', { name: '测试薄荷 50 级 阶段六 闪光' })
    // 走内容包的图像素材，不退回 SVG 画法（jsdom 不解码图片，这里断言结构而非 <img> 本身）
    const object = shiny.closest('.garden-image-object')
    expect(object?.querySelector('.garden-image-specimen')).toBeTruthy()
    expect(object?.querySelector('svg')).toBeNull()
    // 未收录的收藏不占场景位，只在页脚计数
    expect(container.querySelectorAll('.garden-image-object')).toHaveLength(1)
    expect(screen.getByText(/1 项未被当前内容包收录/)).toBeTruthy()
    fireEvent.click(shiny)
    expect(onOpen).toHaveBeenCalledWith('shiny')
  })

  it('场景不渲染领取式徽标或未读红点', () => {
    const { container } = render(<GardenScene specimens={[spec({ shiny: true })]} packId="scene-probe" onOpen={() => {}} />)
    expect(container.querySelector('.garden-new-badge')).toBeNull()
    expect(container.querySelector('.garden-image-feedback')).toBeNull()
  })
})

describe('GardenPanel', () => {
  const snapshot: GardenSnapshot = {
    schemaVersion: 2, ruleVersion: 2, gardenLevel: 9,
    balance: 95, earned: 10, spent: 35,
    specimens: [spec({ instanceId: 'i_1', level: 22 })], changeSignal: null,
  }

  it('头部信息条展示等级、收藏数量与货币', async () => {
    render(<GardenPanel host={host(snapshot)} isOpen height={320} packId="scene-probe" />)
    expect(await screen.findByText(/等级 9 · 1 位伙伴/)).toBeTruthy()
    expect(screen.getByText('测试场景')).toBeTruthy()
    expect(screen.getByText(/95 灵感币/)).toBeTruthy()
  })

  it('空场景仍使用内容包背景并显示题材引导', async () => {
    const empty: GardenSnapshot = { ...snapshot, specimens: null }
    const { container } = render(<GardenPanel host={host(empty)} isOpen height={320} packId="scene-probe" />)
    expect(await screen.findByText(/还没有摆放元素/)).toBeTruthy()
    expect(container.querySelectorAll('.garden-image-background')).toHaveLength(2)
  })

  it('状态 schema 不兼容时明确报错，不尝试绘制', async () => {
    const incompatible: GardenSnapshot = { ...snapshot, schemaVersion: 3 }
    const { container } = render(<GardenPanel host={host(incompatible)} isOpen height={320} packId="scene-probe" />)
    expect(await screen.findByRole('alert')).toHaveTextContent(/不支持状态 schema v3/)
    expect(container.querySelector('.garden-image-world')).toBeNull()
  })

  it('没有登记内容包时给出明确提示，不尝试绘制', async () => {
    dispose()
    const { container } = render(<GardenPanel host={host(snapshot)} isOpen height={320} packId="not-registered" />)
    expect(await screen.findByRole('alert')).toHaveTextContent(/未安装或登记失败/)
    expect(container.querySelector('.garden-image-world')).toBeNull()
    dispose = registerPack(pack)
  })

  it('关闭时不取快照且不挂载任何场景资源', () => {
    const load = vi.fn(() => Promise.resolve(snapshot))
    const { container } = render(<GardenPanel host={{ ...host(snapshot), snapshot: load }} isOpen={false} height={320} packId="scene-probe" />)
    expect(load).not.toHaveBeenCalled()
    expect(container.firstChild).toBeNull()
    expect(container.querySelector('img')).toBeNull()
  })

  it('从商店购买后进入背包，并由用户选择摆放到场景', async () => {
    let current: GardenSnapshot = { ...snapshot, gardenLevel: 0, balance: 120, earned: 0, spent: 0, specimens: [] }
    const purchase = vi.fn(async () => {
      const specimen = spec({ instanceId: 'bought', level: 50, placement: { placed: false, x: 0, y: 0, scale: 1, flipX: false } })
      current = { ...current, balance: 85, spent: 35, specimens: [specimen] }
      return { balance: 85, specimen }
    })
    const place = vi.fn(async (id: string, x: number, y: number, scale: number, flipX: boolean) => {
      current = { ...current, specimens: [spec({ instanceId: id, level: 50, placement: { placed: true, x, y, scale, flipX } })] }
      return current
    })
    render(<GardenPanel host={{ snapshot: async () => current, signal: async () => null, purchase, place, stow: async () => current }} isOpen height={320} packId="scene-probe" />)
    await screen.findByText(/120 灵感币/)
    fireEvent.click(screen.getByRole('button', { name: '商店' }))
    fireEvent.click(screen.getByRole('button', { name: '◆ 35' }))
    expect(await screen.findByText('摆放')).toBeTruthy()
    expect(purchase).toHaveBeenCalledWith('cmd-mint', 35, 50)
    fireEvent.click(screen.getByRole('button', { name: /测试薄荷摆放/ }))
    expect(await screen.findByRole('button', { name: '测试薄荷 50 级 阶段六' })).toBeTruthy()
    expect(place).toHaveBeenCalledWith('bought', expect.any(Number), expect.any(Number), 1, false)
  })
})

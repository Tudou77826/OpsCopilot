import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import GardenScene, { layoutSpecimens } from './GardenScene'
import GardenPanel from './GardenPanel'
import type { GardenPending, GardenSpecimen, GardenSnapshot } from './pack'

// 引入即登记内置呈现包（物种名来自包）。
import './packs'

function spec(overrides: Partial<GardenSpecimen>): GardenSpecimen {
  return {
    speciesId: 'session-tree',
    instanceId: 'i_1',
    level: 1,
    xp: 0,
    shiny: false,
    acquiredAt: '2026-09-01T00:00:00Z',
    lastGrewAt: '2026-09-01T00:00:00Z',
    ...overrides,
  }
}

describe('layoutSpecimens', () => {
  it('最新的一株落在最前排（近带），旧的依次往后排', () => {
    const specs = [
      spec({ instanceId: 'old', acquiredAt: '2026-09-01T00:00:00Z' }),
      spec({ instanceId: 'mid', acquiredAt: '2026-09-02T00:00:00Z' }),
      spec({ instanceId: 'newest', acquiredAt: '2026-09-03T00:00:00Z' }),
    ]
    const placed = layoutSpecimens(specs)
    const byId = new Map(placed.map(item => [item.spec.instanceId, item]))
    // row 越大越近（z 序越高、bottom 越小）。
    expect(byId.get('newest')!.row).toBe(2)
    expect(byId.get('mid')!.row).toBe(1)
    expect(byId.get('old')!.row).toBe(0)
    expect(byId.get('newest')!.bottom).toBeLessThan(byId.get('mid')!.bottom)
    // 近带的株应当比远带同级的株大（纵深）。
    expect(byId.get('newest')!.size).toBeGreaterThan(byId.get('old')!.size)
  })

  it('布局是状态的纯函数：同一份快照两次布局完全一致（不闪变）', () => {
    const specs = Array.from({ length: 7 }, (_, i) =>
      spec({ instanceId: `i_${i}`, level: 3 + i, acquiredAt: `2026-09-0${i + 1}T00:00:00Z` }))
    expect(layoutSpecimens(specs)).toEqual(layoutSpecimens(specs))
  })

  it('位置始终落在场地内（8%–92%，宽株不被边缘裁切）', () => {
    const specs = Array.from({ length: 12 }, (_, i) =>
      spec({ instanceId: `x_${i}`, acquiredAt: `2026-09-10T0${i}:00:00Z` }))
    for (const item of layoutSpecimens(specs)) {
      expect(item.x).toBeGreaterThanOrEqual(8)
      expect(item.x).toBeLessThanOrEqual(92)
      expect(item.bottom).toBeGreaterThanOrEqual(1)
    }
  })
})

describe('GardenScene', () => {
  const pending: GardenPending[] = [{
    kind: 'shiny-discovered',
    instanceId: 'i_shiny',
    speciesId: 'guard-orchid',
    at: '2026-09-03T10:00:00Z',
  }]

  it('每株一个可点按钮，名称来自呈现包；画法按包内资源选择（图优先，缺图回退 SVG）', () => {
    const specimens = [
      spec({ instanceId: 'i_1', level: 22 }),
      spec({ instanceId: 'i_shiny', speciesId: 'guard-orchid', level: 30, shiny: true }),
    ]
    const onOpen = vi.fn()
    render(<GardenScene specimens={specimens} pending={pending} onOpen={onOpen} onDismissPending={() => {}} />)
    const shiny = screen.getByRole('button', { name: '守护兰 30 级 成熟 闪光' })
    const normal = screen.getByRole('button', { name: '会话杉 22 级 开花' })
    // 守护兰在默认包（botanical-image）里有图：走 image 画法，光效收敛为闪星 + 光池（无大光晕）
    expect(shiny.querySelector('.garden-image-specimen img')).toBeTruthy()
    expect(shiny.querySelector('.garden-halo')).toBeNull()
    expect(shiny.querySelectorAll('.garden-sparkle').length).toBeGreaterThan(0)
    // 会话杉未在包内覆盖：回退 SVG 画法（带光晕）
    expect(normal.querySelector('.garden-image-specimen')).toBeNull()
    expect(normal.querySelector('svg')).toBeTruthy()

    const badge = screen.getByRole('button', { name: /新发现的闪光株 守护兰/ })
    expect(badge).toBeTruthy()
    fireEvent.click(normal)
    expect(onOpen).toHaveBeenCalledWith('i_1')
  })

  it('点击新株徽标触发已读回调，且不会同时打开详情', () => {
    const onDismiss = vi.fn()
    const onOpen = vi.fn()
    render(
      <GardenScene
        specimens={[spec({ instanceId: 'i_shiny', speciesId: 'guard-orchid', shiny: true })]}
        pending={pending}
        onOpen={onOpen}
        onDismissPending={onDismiss}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /新发现的闪光株/ }))
    expect(onDismiss).toHaveBeenCalledWith('2026-09-03T10:00:00Z')
    expect(onOpen).not.toHaveBeenCalled()
  })
})

describe('GardenPanel', () => {
  const snapshot: GardenSnapshot = {
    schemaVersion: 1,
    ruleVersion: 1,
    gardenLevel: 9,
    specimens: [spec({ instanceId: 'i_1', level: 22 })],
    pitySinceShiny: 7,
    pending: [],
  }

  it('头部信息条展示等级、株数与保底进度', async () => {
    render(<GardenPanel host={{ snapshot: () => Promise.resolve(snapshot), dismiss: () => Promise.resolve() }} isOpen height={320} />)
    expect(await screen.findByText(/等级 9 · 1 株/)).toBeTruthy()
    expect(screen.getByText('花园')).toBeTruthy()
    expect(screen.getByText(/保底 7\/40/)).toBeTruthy()
  })

  it('空花园显示引导气泡，长出第一株后气泡消失', async () => {
    const empty: GardenSnapshot = { ...snapshot, specimens: null }
    const { unmount } = render(<GardenPanel host={{ snapshot: () => Promise.resolve(empty), dismiss: () => Promise.resolve() }} isOpen height={320} />)
    expect(await screen.findByText(/空花园/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: /会话杉/ })).toBeNull()
    unmount()
    render(<GardenPanel host={{ snapshot: () => Promise.resolve(snapshot), dismiss: () => Promise.resolve() }} isOpen height={320} />)
    await screen.findByText(/等级 9 · 1 株/)
    expect(screen.queryByText(/空花园/)).toBeNull()
  })
})

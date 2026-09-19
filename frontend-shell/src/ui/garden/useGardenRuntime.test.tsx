import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { createGardenPack, type GardenPackManifest } from './contentPack'
import { useGardenRuntime } from './useGardenRuntime'

// 自带测试包，不依赖任何生产内容包：本用例检验的是运行时生命周期，
// 与当前上线的是哪个主题无关。
const manifest: GardenPackManifest = {
  contractVersion: 2, id: 'runtime-probe', version: '1.0.0', name: '运行时探针', runtime: { min: 2, max: 2 }, stateSchema: { min: 2, max: 2 },
  presentation: { title: '探针场景', unit: '个', empty: '空', maxLevel: '满级' },
  stageLabels: ['一', '二', '三', '四', '五', '六'],
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
      name: '探针薄荷', meaning: '探针', stages: ['actor', 'actor', 'actor', 'actor', 'actor', 'actor'],
      anchor: { x: 0.5, y: 0.9 }, hitArea: { x: 0, y: 0, width: 1, height: 1 }, defaultSize: 1,
      placement: { type: 'ambient', zone: 'meadow', layer: 'near' },
      actions: { idle: { format: 'frame-sequence', assets: ['actor', 'actor'], fps: 2, loop: true, staticFrame: 0 } },
      behavior: { durations: { idle: { minMs: 4000, maxMs: 4000 } }, weights: { idle: 1 } },
      reducedMotion: { asset: 'actor' },
    },
  },
}
const pack = createGardenPack(manifest, Object.fromEntries(Object.values(manifest.assets).map(asset => [asset.path, asset.path])))
const input = [{ instanceId: 'mint-1', itemId: 'cmd-mint', slot: pack.scene.slots[0] }]

function RuntimeProbe() {
  const runtime = useGardenRuntime(pack, input)
  return <output data-testid="runtime" data-running={runtime.running} data-reduced={runtime.reducedMotion}>{runtime.actors[0]?.action}</output>
}

function installMotionPreference(reduced: boolean) {
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches: reduced,
    media: '(prefers-reduced-motion: reduce)',
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })))
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('useGardenRuntime lifecycle', () => {
  it('挂载时启动单一 RAF，卸载时立即取消', () => {
    installMotionPreference(false)
    const request = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 41)
    const cancel = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const view = render(<RuntimeProbe />)
    expect(screen.getByTestId('runtime')).toHaveAttribute('data-running', 'true')
    expect(request).toHaveBeenCalledTimes(1)
    view.unmount()
    expect(cancel).toHaveBeenCalledWith(41)
  })

  it('页面隐藏时暂停调度，恢复可见后从当前时刻重新开始', () => {
    installMotionPreference(false)
    let visibility: DocumentVisibilityState = 'visible'
    vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visibility)
    const request = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 52)
    const cancel = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    render(<RuntimeProbe />)
    expect(request).toHaveBeenCalledTimes(1)
    act(() => {
      visibility = 'hidden'
      document.dispatchEvent(new Event('visibilitychange'))
    })
    expect(screen.getByTestId('runtime')).toHaveAttribute('data-running', 'false')
    expect(cancel).toHaveBeenCalledWith(52)
    act(() => {
      visibility = 'visible'
      document.dispatchEvent(new Event('visibilitychange'))
    })
    expect(screen.getByTestId('runtime')).toHaveAttribute('data-running', 'true')
    expect(request).toHaveBeenCalledTimes(2)
  })

  it('系统要求减少动态效果时不创建动画调度器', () => {
    installMotionPreference(true)
    const request = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 63)
    render(<RuntimeProbe />)
    expect(screen.getByTestId('runtime')).toHaveAttribute('data-running', 'false')
    expect(screen.getByTestId('runtime')).toHaveAttribute('data-reduced', 'true')
    expect(request).not.toHaveBeenCalled()
  })
})

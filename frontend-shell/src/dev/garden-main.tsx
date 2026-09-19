import React, { useState } from 'react';
import { createRoot } from 'react-dom/client';
import GardenPanel from '../ui/garden/GardenPanel';
import type { GardenSnapshot } from '../ui/garden/pack';
import '../ui/garden/gardenScene.css';

// 花园视觉调参专用 harness：不走 sidecar，快照内联，昼夜用按钮切 data-theme。
// 访问 /garden-dev.html?theme=light&width=380 可复现窄停靠槽。

const snapshot: GardenSnapshot = {
  schemaVersion: 3,
  ruleVersion: 3,
  gardenLevel: 9,
  balance: 186,
  earned: 240,
  spent: 174,
  specimens: [
    { itemId: 'session-tree', instanceId: 'i1', level: 50, xp: 0, shiny: false, acquiredAt: '2026-08-06T03:00:29Z', lastGrewAt: '2026-08-06T03:00:29Z', milestones: [], placement: { placed: true, x: .2, y: .51, scale: 1, flipX: false } },
    { itemId: 'cmd-mint', instanceId: 'i2', level: 50, xp: 0, shiny: false, acquiredAt: '2026-08-16T03:00:29Z', lastGrewAt: '2026-08-16T03:00:29Z', milestones: [], placement: { placed: true, x: .32, y: .69, scale: 1, flipX: false } },
    { itemId: 'script-vine', instanceId: 'i3', level: 50, xp: 0, shiny: false, acquiredAt: '2026-08-25T03:00:29Z', lastGrewAt: '2026-08-25T03:00:29Z', milestones: [], placement: { placed: true, x: .78, y: .54, scale: 1, flipX: false } },
    { itemId: 'transfer-fern', instanceId: 'i4', level: 50, xp: 0, shiny: false, acquiredAt: '2026-09-01T03:00:29Z', lastGrewAt: '2026-09-01T03:00:29Z', milestones: [], placement: { placed: true, x: .68, y: .7, scale: 1, flipX: false } },
    { itemId: 'guard-orchid', instanceId: 'i5', level: 50, xp: 0, shiny: true, acquiredAt: '2026-09-06T03:00:29Z', lastGrewAt: '2026-09-06T03:00:29Z', milestones: [], placement: { placed: true, x: .79, y: .85, scale: 1, flipX: false } },
    { itemId: 'knowledge-tree', instanceId: 'i6', level: 50, xp: 0, shiny: false, acquiredAt: '2026-09-08T03:00:29Z', lastGrewAt: '2026-09-08T03:00:29Z', milestones: [], placement: { placed: false, x: 0, y: 0, scale: 1, flipX: false } },
  ],
  changeSignal: null,
}

const params = new URLSearchParams(window.location.search)
const initialTheme = params.get('theme') ?? 'dark'
const requestedWidth = Number(params.get('width'))
const previewWidth = Number.isFinite(requestedWidth) && requestedWidth >= 280 ? requestedWidth : undefined
document.documentElement.setAttribute('data-theme', initialTheme)

const Harness: React.FC = () => {
  const [theme, setTheme] = useState(initialTheme)
  const [height, setHeight] = useState(320)
  const toggle = () => {
    const next = theme === 'dark' ? 'light' : 'dark'
    document.documentElement.setAttribute('data-theme', next)
    setTheme(next)
  }
  return (
    <div className="garden-dev-host" data-theme={theme} style={{ minHeight: '100vh' }}>
      <div style={{ position: 'relative', zIndex: 1, padding: '24px' }}>
        <div style={{ display: 'flex', gap: 12, marginBottom: 12 }}>
          <button onClick={toggle} style={{ padding: '6px 14px' }}>切换到{theme === 'dark' ? '白天' : '夜晚'}</button>
          {[260, 320, 400].map(value => (
            <button key={value} onClick={() => setHeight(value)} style={{ padding: '6px 14px', fontWeight: height === value ? 700 : 400 }}>{value}px</button>
          ))}
        </div>
        <div style={{ width: previewWidth, maxWidth: '100%' }}>
          <GardenPanel host={{
            snapshot: () => Promise.resolve(snapshot), signal: () => Promise.resolve(snapshot.changeSignal),
            purchase: () => Promise.reject(new Error('预览页不写入数据')),
            place: () => Promise.resolve(snapshot), stow: () => Promise.resolve(snapshot),
          }} isOpen height={height} />
        </div>
      </div>
    </div>
  )
}

createRoot(document.getElementById('root')!).render(<Harness />)

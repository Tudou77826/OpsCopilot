import React, { useState } from 'react';
import { createRoot } from 'react-dom/client';
import GardenPanel from '../ui/garden/GardenPanel';
import type { GardenSnapshot } from '../ui/garden/pack';
import '../ui/garden/gardenScene.css';

// 花园视觉调参专用 harness：不走 sidecar，快照内联，昼夜用按钮切 data-theme。
// 访问 /garden-dev.html?theme=light&pending=1&width=380 可复现窄停靠槽。

const snapshot: GardenSnapshot = {
  schemaVersion: 1,
  ruleVersion: 1,
  gardenLevel: 9,
  pitySinceShiny: 7,
  specimens: [
    { speciesId: 'session-tree', instanceId: 'i1', level: 22, xp: 140, shiny: false, acquiredAt: '2026-08-06T03:00:29Z', lastGrewAt: '2026-08-06T03:00:29Z', milestones: [] },
    { speciesId: 'cmd-mint', instanceId: 'i2', level: 12, xp: 60, shiny: false, acquiredAt: '2026-08-16T03:00:29Z', lastGrewAt: '2026-08-16T03:00:29Z', milestones: [] },
    { speciesId: 'script-vine', instanceId: 'i3', level: 27, xp: 300, shiny: false, acquiredAt: '2026-08-25T03:00:29Z', lastGrewAt: '2026-08-25T03:00:29Z', milestones: [] },
    { speciesId: 'transfer-fern', instanceId: 'i4', level: 8, xp: 20, shiny: false, acquiredAt: '2026-09-01T03:00:29Z', lastGrewAt: '2026-09-01T03:00:29Z', milestones: [] },
    { speciesId: 'guard-orchid', instanceId: 'i5', level: 30, xp: 0, shiny: true, acquiredAt: '2026-09-06T03:00:29Z', lastGrewAt: '2026-09-06T03:00:29Z', milestones: [] },
    { speciesId: 'knowledge-tree', instanceId: 'i6', level: 18, xp: 90, shiny: false, acquiredAt: '2026-09-08T03:00:29Z', lastGrewAt: '2026-09-08T03:00:29Z', milestones: [] },
  ],
  pending: [],
}

const params = new URLSearchParams(window.location.search)
const initialTheme = params.get('theme') ?? 'dark'
const requestedWidth = Number(params.get('width'))
const previewWidth = Number.isFinite(requestedWidth) && requestedWidth >= 280 ? requestedWidth : undefined
document.documentElement.setAttribute('data-theme', initialTheme)
if (params.get('pending')) {
  snapshot.pending = [{ kind: 'shiny-discovered', instanceId: 'i5', speciesId: 'guard-orchid', at: '2026-09-10T10:00:00Z' }]
}

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
          <GardenPanel host={{ snapshot: () => Promise.resolve(snapshot), dismiss: () => Promise.resolve() }} isOpen height={height} />
        </div>
      </div>
    </div>
  )
}

createRoot(document.getElementById('root')!).render(<Harness />)

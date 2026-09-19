import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import test from 'node:test'

const checker = join(import.meta.dirname, 'check-garden-assets.mjs')

function png(width, height) {
  const data = Buffer.alloc(24)
  data.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])
  data.writeUInt32BE(width, 16)
  data.writeUInt32BE(height, 20)
  return data
}

function fixture(overrides = {}) {
  const root = mkdtempSync(join(tmpdir(), 'garden-assets-'))
  const pack = join(root, 'pack')
  const assets = join(pack, 'assets')
  mkdirSync(assets, { recursive: true })
  writeFileSync(join(assets, 'scene.png'), png(1200, 600))
  writeFileSync(join(assets, 'item.png'), png(512, 512))
  const manifest = {
    contractVersion: 2, id: 'fixture', version: '1.0.0',
    assets: {
      scene: { path: 'assets/scene.png', kind: 'image', bytes: 24, width: 1200, height: 600 },
      item: { path: 'assets/item.png', kind: 'image', bytes: 24, width: 512, height: 512 },
    },
    elements: { item: { stages: ['item', 'item', 'item', 'item', 'item', 'item'], actions: {} } },
    ...overrides,
  }
  writeFileSync(join(pack, 'manifest.json'), JSON.stringify(manifest))
  return root
}

function run(root) {
  return execFileSync(process.execPath, [checker, `--packs-root=${root}`], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] })
}

test('garden asset gate accepts matching bytes and dimensions', t => {
  const root = fixture()
  t.after(() => rmSync(root, { recursive: true, force: true }))
  assert.match(run(root), /fixture@1.0.0 2 assets 48 bytes/)
})

test('garden asset gate rejects stale metadata', t => {
  const root = fixture({
    assets: { scene: { path: 'assets/scene.png', kind: 'image', bytes: 25, width: 1200, height: 600 } },
    elements: { item: { stages: ['scene', 'scene', 'scene', 'scene', 'scene', 'scene'], actions: {} } },
  })
  t.after(() => rmSync(root, { recursive: true, force: true }))
  assert.throws(() => run(root), /bytes 声明 25，实际 24/)
})

test('garden asset gate rejects sprite grids outside the real image', t => {
  const root = fixture({
    assets: { sheet: { path: 'assets/item.png', kind: 'sprite-sheet', bytes: 24, width: 512, height: 512 } },
    elements: { item: { stages: ['sheet', 'sheet', 'sheet', 'sheet', 'sheet', 'sheet'], actions: { move: { format: 'sprite-sheet', asset: 'sheet', frameWidth: 300, frameHeight: 300, frames: 4, columns: 2, rows: 2, fps: 8 } } } },
  })
  t.after(() => rmSync(root, { recursive: true, force: true }))
  assert.throws(() => run(root), /帧网格超出图集/)
})

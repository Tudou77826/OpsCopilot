import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { decodePng, encodePng } from './png.mjs'

test('RGB scene expands to opaque RGBA and roundtrips without pixel changes', () => {
  const source = readFileSync(new URL('./courtyard-sources/day.png', import.meta.url))
  assert.equal(source[25], 2)
  const decoded = decodePng(source)
  assert.equal(decoded.width, 2172)
  assert.equal(decoded.height, 724)
  for (let i = 3; i < decoded.data.length; i += 4) assert.equal(decoded.data[i], 255)
  assert.deepEqual(decodePng(encodePng(decoded)), decoded)
})
test('RGBA sheet preserves transparent margins and opaque subjects', () => {
  const decoded = decodePng(readFileSync(new URL('./courtyard-sources/species.png', import.meta.url)))
  let transparent = 0, opaque = 0
  for (let i = 3; i < decoded.data.length; i += 4) {
    if (decoded.data[i] === 0) transparent++
    if (decoded.data[i] === 255) opaque++
  }
  assert.ok(transparent > 0 && opaque > 0)
  assert.deepEqual(decodePng(encodePng(decoded)), decoded)
})

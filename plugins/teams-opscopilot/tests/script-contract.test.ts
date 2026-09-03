import assert from 'node:assert/strict'
import test from 'node:test'
import { scriptChanges, replayValues } from '../src/script-contract.js'

test('script edits whitelist mutable fields and deleting steps clears legacy commands', () => {
  const result = scriptChanges({ id: '../foreign', name: 'updated', host: 'spoofed', steps: [], commands: [{ content: 'deleted' }] })
  assert.equal('id' in result, false)
  assert.equal('host' in result, false)
  assert.deepEqual(result.steps, [])
  assert.deepEqual(result.commands, [])
})
test('script schema bounds steps, variables and delay before side effects', () => {
  for (const script of [
    { name: '', steps: [] },
    { name: 'test', steps: [{ command: 'x', enabled: true, delay: -1 }] },
    { name: 'test', steps: [{ command: 'x', enabled: 'yes' }] },
    { name: 'test', variables: [{ name: 'a', required: true }, { name: 'a', required: false }] },
    { name: 'test', steps: Array(1001).fill({ command: 'x', enabled: true }) },
  ]) assert.throws(() => scriptChanges(script), { code: 'INVALID_ARGUMENT' })
  assert.throws(() => replayValues({ value: { nested: 'no' } }), { code: 'INVALID_ARGUMENT' })
  assert.throws(() => replayValues({ 'invalid-name': 'no' }), { code: 'INVALID_ARGUMENT' })
})

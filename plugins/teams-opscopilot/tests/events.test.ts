import assert from 'node:assert/strict'
import test from 'node:test'
import { EventJournal } from '../src/events.js'

test('events resume after a snapshot, are bounded and cannot be mutated by readers', () => {
  const journal = new EventJournal(() => 1, 2), snapshot = journal.cursor()
  journal.emit('terminal.opened', 'one', { connectionId: 'conn' })
  const delta = journal.since(snapshot)
  assert.equal(delta.resync, false)
  assert.equal(delta.events[0].sequence, 1)
  delta.events[0].payload.connectionId = 'spoofed'
  assert.equal(journal.since(snapshot).events[0].payload.connectionId, 'conn')
  journal.emit('terminal.closed', 'one'); journal.emit('terminal.opened', 'two')
  assert.equal(journal.since(snapshot).resync, true)
  assert.deepEqual(journal.since(snapshot).events, [])
  assert.equal(journal.since({ ...snapshot, sequence: 1 }).events.length, 2)
})

test('generation and Core instance changes require a fresh authoritative snapshot', () => {
  let generation = 1
  const journal = new EventJournal(() => generation), snapshot = journal.cursor()
  generation++
  assert.equal(journal.since(snapshot).resync, true)
  assert.equal(journal.since({ ...journal.cursor(), instanceId: 'previous-core' }).resync, true)
  assert.equal(journal.since({ ...journal.cursor(), sequence: 999 }).resync, true)
  assert.throws(() => journal.since({ ...journal.cursor(), sequence: -1 }), { code: 'INVALID_ARGUMENT' })
})

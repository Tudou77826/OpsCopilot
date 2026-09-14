import assert from 'node:assert/strict'
import { PassThrough } from 'node:stream'
import test from 'node:test'
import { StdioRpc } from '../src/stdio-rpc.js'

test('pairs out-of-order replies, handles split UTF-8 and isolates event observers', async () => {
  const input = new PassThrough(), output = new PassThrough()
  const rpc = new StdioRpc(input, output)
  const events: unknown[] = []
  rpc.subscribe(() => { throw new Error('observer failed') })
  rpc.subscribe(event => events.push(event))
  try {
    const first = rpc.call('a'), second = rpc.call('b')
    const bytes = Buffer.from('{"jsonrpc":"2.0","method":"notice","params":"中文"}\n{"jsonrpc":"2.0","id":2,"result":"二"}\n{"jsonrpc":"2.0","id":1,"result":"一"}\n')
    for (const byte of bytes) output.write(Buffer.from([byte]))
    assert.deepEqual(await Promise.all([first, second]), ['一', '二'])
    assert.deepEqual(events, [{ method: 'notice', params: '中文' }])
  } finally { rpc.close(); input.destroy(); output.destroy() }
})

test('times out without replay and accepts a later independent response', async () => {
  const input = new PassThrough(), output = new PassThrough(), rpc = new StdioRpc(input, output)
  try {
    await assert.rejects(rpc.call('slow', {}, 10), { code: 'TIMEOUT' })
    output.write('{"jsonrpc":"2.0","id":1,"result":"late"}\n')
    const next = rpc.call('next')
    output.write('{"jsonrpc":"2.0","id":2,"result":true}\n')
    assert.equal(await next, true)
    assert.equal(input.read().toString().split('\n').filter(Boolean).length, 2)
  } finally { rpc.close(); input.destroy(); output.destroy() }
})

test('bounds pending calls and bytes, fails all requests on malformed protocol', async () => {
  const input = new PassThrough(), output = new PassThrough()
  const rpc = new StdioRpc(input, output, () => {}, 128, 1)
  try {
    const first = rpc.call('a')
    const rejected = assert.rejects(first, { code: 'PROTOCOL_MISMATCH' })
    await assert.rejects(rpc.call('b'), { code: 'BUSY' })
    output.write('not JSON\n'); await rejected
    await assert.rejects(rpc.call('c'), { code: 'RUNTIME_UNAVAILABLE' })
  } finally { rpc.close(); input.destroy(); output.destroy() }
  const a = new PassThrough(), b = new PassThrough(), small = new StdioRpc(a, b, () => {}, 80)
  try { await assert.rejects(small.call('large', 'x'.repeat(200)), { code: 'INVALID_ARGUMENT' }) }
  finally { small.close(); a.destroy(); b.destroy() }
})

test('redacts remote errors and rejects outstanding requests on EOF', async () => {
  const input = new PassThrough(), output = new PassThrough(), rpc = new StdioRpc(input, output)
  try {
    const first = rpc.call('secret')
    output.write('{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"password=secret"}}\n')
    await assert.rejects(first, error => error instanceof Error && !error.message.includes('secret'))
    const next = rpc.call('pending')
    const rejected = assert.rejects(next, { code: 'RUNTIME_UNAVAILABLE' })
    output.end(); await rejected
  } finally { rpc.close(); input.destroy(); output.destroy() }
})

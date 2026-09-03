import test from 'node:test'
import assert from 'node:assert/strict'
import { assertCompatible, compatibility, pluginMarker } from '../src/compatibility.js'
const valid = () => ({ product:'OpsCopilot', version:'v2.0.0', marker:pluginMarker, protocol:1, apiRevision:1, sharedConfig:1, installationLifecycle:1, capabilities:[...compatibility.requiredCapabilities] })
test('release labels may evolve while the declared API remains compatible', () => {
  for(const version of ['v1.0.0','v9.0.0','dev']) assert.doesNotThrow(()=>assertCompatible({...valid(),version}))
  assert.doesNotThrow(()=>assertCompatible({...valid(),capabilities:[...valid().capabilities,'future.optional']}))
})
test('missing or unsupported protocol, data, lifecycle and capability versions fail closed', () => {
  for(const bad of [null, {}, {...valid(),protocol:2},{...valid(),apiRevision:0},{...valid(),apiRevision:2},{...valid(),sharedConfig:2},{...valid(),installationLifecycle:undefined},{...valid(),capabilities:[]},{...valid(),version:''}])
    assert.throws(()=>assertCompatible(bad),{code:'PROTOCOL_MISMATCH'})
  for(const removed of compatibility.requiredCapabilities) assert.throws(()=>assertCompatible({...valid(),capabilities:valid().capabilities.filter(c=>c!==removed)}),{code:'PROTOCOL_MISMATCH'})
})

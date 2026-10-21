import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { loadTrialModel } from './assistant-real-model.mjs';

async function config(t, value) {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'assistant-model-test-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const filename = path.join(directory, 'config.json');
  await writeFile(filename, JSON.stringify(value));
  return filename;
}

test('copies model settings only and follows application model precedence', async t => {
  const filename = await config(t, { llm: { BaseURL: 'https://model.example/v1', APIKey: 'test-only-secret', ComplexModel: 'complex', Model: 'legacy', FastModel: 'fast', unknown: 'ignored' }, sessions: ['business-server'], docs: { dir: '/business' } });
  assert.deepEqual(await loadTrialModel(filename), { BaseURL: 'https://model.example/v1', APIKey: 'test-only-secret', Model: 'complex', ComplexModel: 'complex' });
});

test('supports legacy and fast models without requiring an invented API key', async t => {
  for (const llm of [{ Model: 'legacy', FastModel: 'fast' }, { FastModel: 'fast' }]) {
    const filename = await config(t, { llm: { ...llm, BaseURL: 'http://127.0.0.1:8080/v1' } });
    const result = await loadTrialModel(filename);
    assert.equal(result.Model, llm.Model || llm.FastModel);
    assert.equal(result.APIKey, '');
  }
});

test('rejects invalid settings without reflecting secrets', async t => {
  await assert.rejects(loadTrialModel('relative.json'), /absolute path/);
  for (const llm of [undefined, { BaseURL: 'https://model.example', Model: 4 }, { BaseURL: 'https://name:test-only-secret@model.example', Model: 'x' }, { BaseURL: 'file:///test-only-secret', Model: 'x' }, { BaseURL: 'test-only-secret', Model: 'x' }, { BaseURL: 'https://model.example', Model: 'x', APIKey: {} }]) {
    const filename = await config(t, { llm });
    await assert.rejects(loadTrialModel(filename), error => !error.message.includes('test-only-secret'));
  }
});

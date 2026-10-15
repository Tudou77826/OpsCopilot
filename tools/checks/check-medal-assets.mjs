import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { decodePng } from '../pixel/png.mjs';

export const medalKinds = ['knowledge-archive','script-created','quick-command-created','case-completed','knowledge-read','script-used','quick-command-used','active-day'];
export const medalTiers = ['locked','bronze','silver','gold'];
export function validateMedalAssets(manifest, root, load = path => readFileSync(path)) {
  const expected = medalKinds.flatMap(kind => medalTiers.map(tier => `medal-${kind}-${tier}`));
  if (manifest?.version !== 1 || manifest.id !== 'medals-v2' || manifest.size !== 128 || Object.keys(manifest).sort().join() !== 'assets,id,size,version' || Object.keys(manifest.assets ?? {}).sort().join() !== expected.sort().join()) throw new Error('Invalid complete medal atlas');
  let bytes = 0;
  const contents = new Set();
  for (const id of expected) {
    if (manifest.assets[id] !== `${id}.png`) throw new Error(`Unsafe medal path: ${id}`);
    const data = load(resolve(root, manifest.assets[id]));
    if (data.length > 70000) throw new Error(`Medal exceeds budget: ${id}`);
    const image = decodePng(data);
    if (image.width !== 128 || image.height !== 128) throw new Error(`Invalid medal canvas: ${id}`);
    let visible = 0, transparent = 0;
    for (let y = 0; y < 128; y++) for (let x = 0; x < 128; x++) {
      const alpha = image.data[(y * 128 + x) * 4 + 3];
      if (alpha === 0) transparent++;
      if (alpha >= 128) {
        visible++;
        if (x < 4 || y < 4 || x >= 124 || y >= 124) throw new Error(`Clipped medal: ${id}`);
      }
    }
    if (visible < 3000 || transparent < 1000) throw new Error(`Invalid medal silhouette: ${id}`);
    const fingerprint = Buffer.from(image.data).toString('base64');
    if (contents.has(fingerprint)) throw new Error(`Duplicate medal art: ${id}`);
    contents.add(fingerprint); bytes += data.length;
  }
  if (bytes > 1500000) throw new Error('Medal atlas exceeds budget');
  return { assets: expected.length, bytes };
}
export function checkMedalAssets() {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../frontend-shell/src/companion/medals/v2');
  const result = validateMedalAssets(JSON.parse(readFileSync(resolve(root,'manifest.json'),'utf8')), root);
  console.log(`medals-v2: ${result.assets} assets, ${result.bytes} bytes`);
}

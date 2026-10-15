/** Reproducible normalization of the reviewed AI originals. No generated
 * business rules or runtime downloads; geometry/palette are fixed by recipe. */
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { decodePng, encodePng } from './png.mjs';
import { normalizePixels } from './normalize.mjs';
import { validateCompanionPack } from '../checks/check-companion-assets.mjs';

const recipePath = resolve(process.argv[2]), recipe = JSON.parse(readFileSync(recipePath, 'utf8'));
const sourceRoot = dirname(recipePath), output = resolve(process.argv[3]);
const palette = recipe.palette;
mkdirSync(output, { recursive: true });
const crop = (image, x, y, width, height) => {
  const data = new Uint8ClampedArray(width * height * 4);
  for (let row = 0; row < height; row++) data.set(image.data.subarray(((y + row) * image.width + x) * 4, ((y + row) * image.width + x + width) * 4), row * width * 4);
  return { width, height, data };
};
function bounds(image, retainBody = false) {
  if (retainBody) {
    // Reviewed character cells contain one connected body. Remove isolated AI
    // alpha specks by connectivity, never by RGB/background brightness.
    image = { ...image, data: new Uint8ClampedArray(image.data) };
    const visited = new Uint8Array(image.width * image.height);
    let largest = [];
    for (let i = 0; i < visited.length; i++) if (!visited[i] && image.data[i * 4 + 3] >= 128) {
      const component = [i]; visited[i] = 1;
      for (let cursor = 0; cursor < component.length; cursor++) {
        const position = component[cursor], x = position % image.width, y = Math.floor(position / image.width);
        for (let dy = -1; dy <= 1; dy++) for (let dx = -1; dx <= 1; dx++) {
          const nx = x + dx, ny = y + dy, next = ny * image.width + nx;
          if (nx >= 0 && nx < image.width && ny >= 0 && ny < image.height && !visited[next] && image.data[next * 4 + 3] >= 128) { visited[next] = 1; component.push(next); }
        }
      }
      if (component.length > largest.length) largest = component;
    }
    const body = new Set(largest);
    for (let i = 0; i < visited.length; i++) if (!body.has(i)) image.data.fill(0, i * 4, i * 4 + 4);
  }
  let left = image.width, top = image.height, right = -1, bottom = -1;
  for (let y = 0; y < image.height; y++) for (let x = 0; x < image.width; x++) if (image.data[(y * image.width + x) * 4 + 3] >= 128) {
    left = Math.min(left, x); top = Math.min(top, y); right = Math.max(right, x); bottom = Math.max(bottom, y);
  }
  if (right < 0) throw new Error('Empty AI source cell');
  return crop(image, left, top, right - left + 1, bottom - top + 1);
}
function normalized(image, width, height) {
  // Explicit proportional resampling: square padding avoids aspect distortion.
  const side = Math.max(image.width, image.height);
  const square = { width: side, height: side, data: new Uint8ClampedArray(side * side * 4) };
  const x = Math.floor((side - image.width) / 2), y = side - image.height;
  for (let row = 0; row < image.height; row++) square.data.set(image.data.subarray(row * image.width * 4, (row + 1) * image.width * 4), ((y + row) * side + x) * 4);
  return normalizePixels(square, width, height, palette);
}
const assets = {}, stages = {};
function save(id, image, role) {
  const file = `${id}.png`;
  const encoded = encodePng(image), target = resolve(output, file);
  if (existsSync(target)) { if (!readFileSync(target).equals(encoded)) throw new Error(`Refusing different existing asset ${target}`); }
  else writeFileSync(target, encoded, { flag: 'wx' });
  assets[id] = { path: file, role, width: image.width, height: image.height };
}
const sheet = decodePng(readFileSync(resolve(sourceRoot, recipe.sheet)));
if (sheet.width % 8 || sheet.height % 3 || sheet.width / 8 !== sheet.height / 3) throw new Error('Reviewed sprite grid must contain 8 x 3 square cells');
for (const [row, stage] of ['child', 'growing', 'adult'].entries()) {
  const cells = Array.from({ length: 8 }, (_, column) => bounds(crop(sheet, column * sheet.width / 8, row * sheet.height / 3, sheet.width / 8, sheet.height / 3), true));
  const maxSide = Math.max(...cells.map(cell => Math.max(cell.width, cell.height)));
  const targetSide = recipe.stageSizes[row];
  stages[stage] = {};
  for (const [actionIndex, action] of ['idle', 'happy', 'greet', 'sleep'].entries()) {
    const frames = [];
    for (let frame = 0; frame < 2; frame++) {
      const cell = cells[actionIndex * 2 + frame];
      // All actions in a stage use the same source scale and stable foot anchor.
      const square = { width: maxSide, height: maxSide, data: new Uint8ClampedArray(maxSide * maxSide * 4) };
      const x = Math.floor((maxSide - cell.width) / 2), y = maxSide - cell.height;
      for (let sy = 0; sy < cell.height; sy++) square.data.set(cell.data.subarray(sy * cell.width * 4, (sy + 1) * cell.width * 4), ((y + sy) * maxSide + x) * 4);
      const pixels = normalizePixels(square, targetSide, targetSide, palette);
      const canvas = { width: 64, height: 64, data: new Uint8ClampedArray(64 * 64 * 4) };
      const dx = Math.floor((64 - targetSide) / 2), dy = 56 - targetSide;
      for (let sy = 0; sy < pixels.height; sy++) canvas.data.set(pixels.data.subarray(sy * pixels.width * 4, (sy + 1) * pixels.width * 4), ((dy + sy) * 64 + dx) * 4);
      const id = `${stage}-${action}-${frame}`; save(id, canvas, 'character'); frames.push(id);
    }
    stages[stage][action] = { frames, fps: action === 'idle' || action === 'sleep' ? 2 : 4, anchor: { x: 32, y: 56 } };
    if (readFileSync(resolve(output, `${frames[0]}.png`)).equals(readFileSync(resolve(output, `${frames[1]}.png`)))) throw new Error(`${stage}/${action}: source poses must be distinct`);
  }
}
const rgb = hex => [Number.parseInt(hex.slice(1, 3), 16), Number.parseInt(hex.slice(3, 5), 16), Number.parseInt(hex.slice(5, 7), 16), 255];
for (const [kind, file] of Object.entries(recipe.icons)) {
  const symbol = normalized(bounds(decodePng(readFileSync(resolve(sourceRoot, file)))), 20, 20);
  for (const [tier, color] of Object.entries(recipe.medalBorders)) {
    const canvas = { width: 32, height: 32, data: new Uint8ClampedArray(32 * 32 * 4) };
    const pixel = (x, y, hex) => canvas.data.set(rgb(hex), (y * 32 + x) * 4);
    for (let y = 1; y <= 30; y++) for (let x = 2; x <= 29; x++) {
      const inside = y <= 25 ? x >= (y < 4 ? 5 : 2) && x <= (y < 4 ? 26 : 29) : Math.abs(x - 15.5) <= 30 - y;
      if (inside) pixel(x, y, color);
    }
    for (let y = 4; y <= 25; y++) for (let x = 5; x <= 26; x++) pixel(x, y, '#203247');
    for (let y = 0; y < 20; y++) for (let x = 0; x < 20; x++) if (symbol.data[(y * 20 + x) * 4 + 3]) canvas.data.set(symbol.data.subarray((y * 20 + x) * 4, (y * 20 + x + 1) * 4), ((y + 5) * 32 + x + 6) * 4);
    save(`medal-${kind}-${tier}`, canvas, 'icon');
  }
}
const background = decodePng(readFileSync(resolve(sourceRoot, recipe.background)));
const [bx, by, bw, bh] = recipe.backgroundCrop;
if (bx < 0 || by < 0 || bx + bw > background.width || by + bh > background.height) throw new Error('Invalid reviewed background crop');
save('profile-background', normalizePixels(crop(background, bx, by, bw, bh), 320, 180, palette), 'background');
const manifest = { contractVersion: 1, id: 'lumen-v1', speciesId: 'lumen', style: 'pixel', palette, palettes: recipe.palettes, assets, stages };
validateCompanionPack(manifest, output);
const manifestPath = resolve(output, 'manifest.json'), manifestText = JSON.stringify(manifest, null, 2);
if (existsSync(manifestPath)) { if (readFileSync(manifestPath, 'utf8') !== manifestText) throw new Error('Refusing a different existing manifest'); }
else writeFileSync(manifestPath, manifestText, { flag: 'wx' });
console.log(`Normalized and validated ${Object.keys(assets).length} assets in ${output}`);

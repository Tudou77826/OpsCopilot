import manifest from './packs/lumen-v1/manifest.json';
import { images } from './packs/lumen-v1/catalog';
import type { PixelContentPack } from './content';
import type { CompanionPalette } from './types';
import { rosterPacks } from './rosterAssets';
import { createAssetCache } from './assetCache';

export const lumenPack = manifest as unknown as PixelContentPack;
export function packForSpecies(species = 'lumen') { return (rosterPacks[species] ?? rosterPacks.lumen).pack; }
const owners = new Map(Object.values(rosterPacks).flatMap(entry => Object.keys(entry.images).map(id => [id, entry] as const)));
export function assetUrl(id: string): string {
  const owner = owners.get(id);
  const asset = owner?.pack.assets[id] ?? lumenPack.assets[id];
  const url = asset && (owner?.images[id] ?? images[id]);
  if (!url) throw new Error(`伙伴素材不存在：${id}`);
  return url;
}
export function recolorPixels(data: Uint8ClampedArray, from: string[], to: string[]): Uint8ClampedArray {
  const colors = new Map(from.map((hex, index) => [Number.parseInt(hex.slice(1), 16), Number.parseInt(to[index].slice(1), 16)]));
  const output = new Uint8ClampedArray(data);
  for (let i = 0; i < output.length; i += 4) {
    if (!output[i + 3]) continue;
    const target = colors.get((output[i] << 16) | (output[i + 1] << 8) | output[i + 2]);
    if (target !== undefined) { output[i] = (target >> 16) & 255; output[i + 1] = (target >> 8) & 255; output[i + 2] = target & 255; }
  }
  return output;
}
/** Change only chromatic hue. Preserve alpha, saturation and brightness at every source pixel. */
export function recolorNaturalPixels(data: Uint8ClampedArray, hue: number): Uint8ClampedArray {
  const output = new Uint8ClampedArray(data), h=hue/60, ramp=1-Math.abs(h%2-1);
  for(let i=0;i<output.length;i+=4) {
    if(!output[i+3]) continue;
    const max=Math.max(output[i],output[i+1],output[i+2]),min=Math.min(output[i],output[i+1],output[i+2]);
    if(max-min<30||max<65||min>205) continue;
    const c=max-min,x=c*ramp;
    output[i]=Math.round(min+(h<1||h>=5?c:h<2||h>=4?x:0));
    output[i+1]=Math.round(min+(h>=1&&h<3?c:h<1||h>=3&&h<4?x:0));
    output[i+2]=Math.round(min+(h>=3&&h<5?c:h>=2&&h<3||h>=5?x:0));
  }
  return output;
}
const variants = createAssetCache<string>(128);
export function paletteAsset(id: string, palette: CompanionPalette): Promise<string> {
  const key = `${id}:${palette}`;
  return variants.get(key, () => new Promise<string>((resolve, reject) => {
    const image = new Image();
    image.onload = () => {
      try {
        if (palette === 'mint') { resolve(assetUrl(id)); return; }
        const canvas = document.createElement('canvas'); canvas.width = image.naturalWidth; canvas.height = image.naturalHeight;
        const context = canvas.getContext('2d'); if (!context) throw new Error('浏览器无法绘制伙伴配色');
        context.drawImage(image, 0, 0);
        const pixels = context.getImageData(0, 0, canvas.width, canvas.height);
        const pack = owners.get(id)?.pack ?? lumenPack;
        pixels.data.set(pack.contractVersion===2 ? recolorNaturalPixels(pixels.data,pack.paletteHues[palette]!) : recolorPixels(pixels.data, pack.palette, pack.palettes[palette]));
        context.putImageData(pixels, 0, 0); resolve(canvas.toDataURL('image/png'));
      } catch (error) { reject(error); }
    };
    image.onerror = () => { reject(new Error('伙伴素材加载失败')); };
    image.src = assetUrl(id);
  }));
}

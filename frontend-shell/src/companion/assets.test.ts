import { expect, it } from 'vitest';
import { recolorPixels, recolorNaturalPixels, lumenPack, assetUrl } from './assets';
it('recolors registered palettes without changing alpha or unlisted pixels', () => {
 const pixels = new Uint8ClampedArray([109,199,163,255,109,199,163,0,32,50,71,255,1,2,3,255]);
 const mapped = recolorPixels(pixels, lumenPack.palette, lumenPack.palettes.amber);
 expect(Array.from(mapped)).toEqual([222,164,91,255,109,199,163,0,32,50,71,255,1,2,3,255]);
 expect(pixels[0]).toBe(109);
 expect(() => assetUrl('unregistered')).toThrow();
});
it('keeps shading and soft alpha when applying a full-color hue variant',()=>{
 const pixels=new Uint8ClampedArray([200,120,40,137,170,100,30,255,245,240,225,200,1,2,3,0]);
 const mapped=recolorNaturalPixels(pixels,272);
 expect(mapped[3]).toBe(137);expect(mapped[7]).toBe(255);
 expect(Math.max(...mapped.slice(0,3))).toBe(200);expect(Math.min(...mapped.slice(0,3))).toBe(40);
 expect(Math.max(...mapped.slice(4,7))).toBe(170);expect(Math.min(...mapped.slice(4,7))).toBe(30);
 expect(Array.from(mapped.slice(8))).toEqual(Array.from(pixels.slice(8)));
 expect(mapped.slice(0,3)).not.toEqual(pixels.slice(0,3));
});

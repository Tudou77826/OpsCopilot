import {expect,it} from 'vitest';
import {roamingRange} from './ambientMotion';
it('keeps the complete silhouette inside the scene and clear of medals',()=>{
 for(const width of [300,420,560])for(const scale of [160/216,188/216,1]) {
  const range=roamingRange(width,Math.min(320,width),scale),radius=Math.min(320,width)*scale*.91/2;
  if(range.max>0){expect(range.center+range.min-radius).toBeCloseTo(12);expect(range.center+range.max+radius).toBeCloseTo(width-72);}
  expect(range.min).toBeLessThanOrEqual(0);expect(range.max).toBeGreaterThanOrEqual(0);
 }
 expect(roamingRange(200,320,1).max).toBe(0);
});

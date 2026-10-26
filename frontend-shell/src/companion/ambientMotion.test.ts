import {expect,it} from 'vitest';
import {roamingRange} from './ambientMotion';
it('keeps the complete silhouette inside a centered floor below the medals',()=>{
 for(const width of [300,420,560])for(const scale of [160/216,188/216,1]) {
  const range=roamingRange(width,Math.min(320,width),scale),radius=Math.min(320,width)*scale*.91/2;
  expect(range.center).toBe(width/2);
  if(range.max>0){expect(range.center+range.min-radius).toBeCloseTo(12);expect(range.center+range.max+radius).toBeCloseTo(width-12);}
  expect(range.min).toBeLessThanOrEqual(0);expect(range.max).toBeGreaterThanOrEqual(0);
 }
 expect(roamingRange(200,320,1).max).toBe(0);
});

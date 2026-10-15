import { visibleBounds } from './sheet.mjs';

/** Align a planted foot baseline, then match the lower torso rather than moving appendages. */
export function plantedRegistration(reference, image) {
  const base=visibleBounds(reference), bounds=visibleBounds(image);
  const dy=base.bottom-bounds.bottom, startY=Math.round(base.top+(base.bottom-base.top)*.55);
  let best={dx:0,dy}, minimum=Infinity;
  for(let dx=-48;dx<=48;dx++) {
    let cost=0;
    for(let y=startY;y<=base.bottom;y+=2) for(let x=Math.max(0,base.left-8);x<=Math.min(reference.width-1,base.right+8);x+=2) {
      const sx=x-dx,sy=y-dy, a=(y*reference.width+x)*4;
      const b=(sy*image.width+sx)*4, valid=sx>=0&&sy>=0&&sx<image.width&&sy<image.height;
      const ra=reference.data[a+3]/255,ia=valid?image.data[b+3]/255:0;
      cost+=(ra-ia)**2*3;
      if(ra>.5&&ia>.5) for(let c=0;c<3;c++) cost+=((reference.data[a+c]-image.data[b+c])/255)**2;
    }
    // Choose the smallest correction where costs are effectively equal.
    cost+=Math.abs(dx)*.002;
    if(cost<minimum){minimum=cost;best={dx,dy};}
  }
  return best;
}
export function translateRgba(image, {dx,dy}) {
  const data=new Uint8ClampedArray(image.data.length);
  for(let y=0;y<image.height;y++) for(let x=0;x<image.width;x++) {
    const sx=x-dx,sy=y-dy;
    if(sx>=0&&sy>=0&&sx<image.width&&sy<image.height) data.set(image.data.subarray((sy*image.width+sx)*4,(sy*image.width+sx+1)*4),(y*image.width+x)*4);
  }
  return {...image,data};
}

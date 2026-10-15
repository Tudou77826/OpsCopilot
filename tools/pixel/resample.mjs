/** Dominant-color sampling for approved-aspect-ratio pixel candidates.
 * No background deletion, automatic outlines, dithering or browser globals.
 */
export function resampleMode(source,width,height) {
  const output={width,height,data:new Uint8ClampedArray(width*height*4)};
  for(let y=0;y<height;y++) for(let x=0;x<width;x++) {
    const left=Math.floor(x*source.width/width),right=Math.max(left+1,Math.floor((x+1)*source.width/width));
    const top=Math.floor(y*source.height/height),bottom=Math.max(top+1,Math.floor((y+1)*source.height/height));
    const groups=new Map();let best,maximum=0,transparent=0;
    for(let sy=top;sy<Math.min(bottom,source.height);sy++) for(let sx=left;sx<Math.min(right,source.width);sx++) {
      const offset=(sy*source.width+sx)*4,p=source.data;
      if(p[offset+3]===0){transparent++;continue;}
      const key=((p[offset]>>3)<<10)|((p[offset+1]>>3)<<5)|(p[offset+2]>>3);
      const group=groups.get(key)??{n:0,r:0,g:0,b:0};
      group.n++;group.r+=p[offset];group.g+=p[offset+1];group.b+=p[offset+2];groups.set(key,group);
      if(group.n>maximum){maximum=group.n;best=group;}
    }
    if(best && maximum>transparent) output.data.set([Math.round(best.r/best.n),Math.round(best.g/best.n),Math.round(best.b/best.n),255],(y*width+x)*4);
  }
  return output;
}

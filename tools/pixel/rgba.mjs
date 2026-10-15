/** Bilinear sampling in premultiplied alpha. No palette quantization or alpha threshold. */
export function sampleRgba(source, x, y) {
  const x0=Math.floor(x), y0=Math.floor(y), output=[0,0,0,0];
  for(let dy=0;dy<2;dy++) for(let dx=0;dx<2;dx++) {
    const sx=x0+dx,sy=y0+dy;
    if(sx<0||sy<0||sx>=source.width||sy>=source.height) continue;
    const weight=(dx ? x-x0 : 1-(x-x0))*(dy ? y-y0 : 1-(y-y0));
    const index=(sy*source.width+sx)*4,alpha=source.data[index+3]*weight;
    output[3]+=alpha;
    for(let c=0;c<3;c++) output[c]+=source.data[index+c]*alpha;
  }
  if(output[3]>0) for(let c=0;c<3;c++) output[c]/=output[3];
  return output;
}

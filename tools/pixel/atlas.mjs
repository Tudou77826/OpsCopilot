// Reviewed AI grids can have uneven gutters. Find the lowest-alpha seam near each grid line.
function seams(projection,count) {
 const result=[0],cell=projection.length/count;
 for(let index=1;index<count;index++) {
  const center=index*cell,start=Math.floor(center-cell*.16),end=Math.ceil(center+cell*.16);
  let best=Infinity,positions=[];
  for(let p=start;p<=end;p++) {
   let value=0;for(let n=-3;n<=3;n++)value+=projection[p+n]??0;
   if(value<best){best=value;positions=[p];}else if(value===best)positions.push(p);
  }
  result.push(Math.round(positions.reduce((sum,p)=>sum+p,0)/positions.length));
 }
 result.push(projection.length);return result;
}
export function atlasRegions(sheet,columns=8,rows=4) {
 const y=new Array(sheet.height).fill(0);
 for(let py=0;py<sheet.height;py++)for(let x=0;x<sheet.width;x++)y[py]+=sheet.data[(py*sheet.width+x)*4+3]>=128?1:0;
 const ys=seams(y,rows),regions=[];
 for(let row=0;row<rows;row++) {
  const x=new Array(sheet.width).fill(0);
  for(let py=ys[row];py<ys[row+1];py++)for(let px=0;px<sheet.width;px++)x[px]+=sheet.data[(py*sheet.width+px)*4+3]>=128?1:0;
  const xs=seams(x,columns);
  for(let column=0;column<columns;column++)regions.push({left:xs[column],right:xs[column+1],top:ys[row],bottom:ys[row+1]});
 }
 return regions;
}

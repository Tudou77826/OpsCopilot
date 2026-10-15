// Crop one grid cell and retain the main body, decorations and soft alpha.
export function extractFrame(sheet,column,row,columns=4,region) {
 const left=region?.left??Math.floor(column*sheet.width/columns),top=region?.top??Math.floor(row*sheet.height/4);
 const width=(region?.right??Math.floor((column+1)*sheet.width/columns))-left,height=(region?.bottom??Math.floor((row+1)*sheet.height/4))-top,data=new Uint8ClampedArray(width*height*4);
 for(let y=0;y<height;y++) data.set(sheet.data.subarray(((top+y)*sheet.width+left)*4,((top+y)*sheet.width+left+width)*4),y*width*4);
 const image={width,height,data},seen=new Uint8Array(width*height),components=[];
 for(let start=0;start<seen.length;start++) if(!seen[start]&&data[start*4+3]>=32) {
  const points=[start];seen[start]=1;
  for(let cursor=0;cursor<points.length;cursor++) {
   const point=points[cursor],x=point%width,y=Math.floor(point/width);
   for(let dy=-1;dy<=1;dy++) for(let dx=-1;dx<=1;dx++) {
    const sx=x+dx,sy=y+dy,next=sy*width+sx;
    if(sx>=0&&sy>=0&&sx<width&&sy<height&&!seen[next]&&data[next*4+3]>=32){seen[next]=1;points.push(next);}
   }
  }
  components.push(points);
 }
 components.sort((a,b)=>b.length-a.length);
 if(!components.length) throw new Error('Empty AI frame');
 // Measure the body, but retain nearby decorations and original soft edges.
 const body=components[0],xs=body.map(p=>p%width),ys=body.map(p=>Math.floor(p/width));
 const bounds={left:Math.min(...xs),right:Math.max(...xs),top:Math.min(...ys),bottom:Math.max(...ys)};
 const remove=new Uint8Array(width*height);
 for(const points of components.slice(1)) {
  const remote=region&&points.every(p=>p%width<bounds.left-12||p%width>bounds.right+12||Math.floor(p/width)<bounds.top-12||Math.floor(p/width)>bounds.bottom+12);
  if(remote||points.length<Math.max(12,body.length*.0008))for(const point of points)remove[point]=1;
 }
 for(let p=0;p<remove.length;p++) if(remove[p]) data.fill(0,p*4,p*4+4);
 return {...image,bounds};
}

export function visibleBounds(image) {
 const bounds={left:image.width,right:0,top:image.height,bottom:0};
 for(let y=0;y<image.height;y++) for(let x=0;x<image.width;x++) if(image.data[(y*image.width+x)*4+3]>=32){bounds.left=Math.min(bounds.left,x);bounds.right=Math.max(bounds.right,x);bounds.top=Math.min(bounds.top,y);bounds.bottom=Math.max(bounds.bottom,y);}
 return bounds;
}

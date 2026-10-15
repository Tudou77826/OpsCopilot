import React, {useEffect,useState} from 'react';
import {assetUrl,packForSpecies,paletteAsset} from '../../companion/assets';
import type {CompanionPalette} from '../../companion/types';
import './companion.css';

/** A static identity thumbnail: crop canvas padding, independent of scene/growth zoom. */
export function CompanionAvatar({species='lumen',palette='mint',name='微光',level}:{
  species?:string;palette?:CompanionPalette;name?:string;level?:number;
}) {
  const pack=packForSpecies(species),id=pack.stages.child.idle.frames[0],native=pack.assets[id];
  const key=id+':'+palette;
  const [loaded,setLoaded]=useState<{key:string;url:string}>(),[failed,setFailed]=useState(false);
  useEffect(()=>{
    let live=true;setFailed(false);
    void paletteAsset(id,palette).then(url=>{if(live)setLoaded({key,url});}).catch(()=>{if(live)setFailed(true);});
    return()=>{live=false;};
  },[id,palette]);
  if(failed)return <span role="status">伙伴图像暂不可用</span>;
  return <svg className="oc-companion-avatar" role="img" aria-label={name+(level?' · Lv.'+level:'')}
    data-species={species} data-level={level} data-palette={palette}
    viewBox={pack.contractVersion===2?'24 32 208 216':`0 0 ${native.width} ${native.height}`}>
    <image href={loaded?.key===key?loaded.url:assetUrl(id)} width={native.width} height={native.height} onError={()=>setFailed(true)}/>
  </svg>;
}

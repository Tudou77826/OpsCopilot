/** Keep every visible pose inside its stage, independently of the medal dock. */
export function roamingRange(sceneWidth: number, canvasWidth: number, stageScale: number, silhouetteRatio = .91) {
  const left=12,right=Math.max(left,sceneWidth-12),center=(left+right)/2;
  const radius=canvasWidth*stageScale*silhouetteRatio/2;
  const distance=Math.max(0,(right-left)/2-radius);
  return {center,min:-distance,max:distance};
}

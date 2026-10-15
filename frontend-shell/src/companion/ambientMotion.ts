/** Keep the full silhouette on the ground, leaving the medal column free. */
export function roamingRange(sceneWidth: number, canvasWidth: number, stageScale: number) {
  const left=12,right=Math.max(left,sceneWidth-72),center=(left+right)/2;
  const radius=canvasWidth*stageScale*.91/2;
  const distance=Math.max(0,(right-left)/2-radius);
  return {center,min:-distance,max:distance};
}

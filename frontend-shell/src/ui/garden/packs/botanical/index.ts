import { createGardenPack, type GardenPackManifest } from '../../contentPack'
import manifestData from './manifest.json'
import dayScene from './assets/day.webp'
import nightScene from './assets/night.webp'
import fernImage from './assets/fern.webp'
import mintImage from './assets/mint.webp'
import orchidImage from './assets/orchid.webp'
import orchidShinyImage from './assets/orchid-shiny.webp'

/**
 * 官方绘本内容包。manifest.json 只含数据；本文件仅把构建器产生的资源 URL
 * 注入清单。增加动物、建筑或节日包时复用同一 createGardenPack，不新增渲染器。
 */
export const botanicalPack = createGardenPack(
  manifestData as unknown as GardenPackManifest,
  {
    'assets/day.webp': dayScene,
    'assets/night.webp': nightScene,
    'assets/fern.webp': fernImage,
    'assets/mint.webp': mintImage,
    'assets/orchid.webp': orchidImage,
    'assets/orchid-shiny.webp': orchidShinyImage,
  },
)

import { createGardenPack, type GardenPackManifest } from '../../contentPack'
import manifestData from './manifest.json'
import { pixelHabitatSources } from './assets.generated'

/**
 * OpsCopilot 首套完整收藏主题。题材、动作与像素资源全部来自清单，
 * 运行时不识别植物、动物或具体物种。
 */
export const pixelHabitatPack = createGardenPack(
  manifestData as unknown as GardenPackManifest,
  pixelHabitatSources,
)

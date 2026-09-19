import { createGardenPack, type GardenPackManifest } from '../../contentPack'
import manifest from './manifest.json'
import { courtyardSources } from './assets.generated'

export const woodlandCourtyardPack = createGardenPack(manifest as unknown as GardenPackManifest, courtyardSources)

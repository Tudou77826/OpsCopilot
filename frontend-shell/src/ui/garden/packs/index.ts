import { registerPack } from '../pack'
import { woodlandCourtyardPack } from './woodlandCourtyard'

/**
 * 内置内容包登记。所有包都先经过 createGardenPack 的契约、资源与预算校验；
 * registerPack 不允许同 id 静默覆盖。
 */
const disposeCourtyard = registerPack(woodlandCourtyardPack)
const hot = (import.meta as ImportMeta & { hot?: { dispose(callback: () => void): void } }).hot
hot?.dispose(disposeCourtyard)

export { woodlandCourtyardPack }

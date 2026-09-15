import { registerPack } from '../pack'
import { floraPack } from './flora'
import { botanicalPack } from './botanical'

/**
 * 内置呈现包登记。运行时通过 registerPack 认识它们；阶段三再加"用户数据目录里的本地包"，
 * 那条路径要过校验（清单可解析、画法内建、资源齐全、schemaVersion 匹配）后再登记。
 */
registerPack(floraPack)
registerPack(botanicalPack)

export { floraPack, botanicalPack }

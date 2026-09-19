import React from 'react'
import ContentGardenScene from './ContentGardenScene'
import { DEFAULT_PACK_ID, getPack, type GardenSpecimen } from './pack'
import './gardenScene.css'

export interface GardenSceneProps {
  specimens: GardenSpecimen[]
  packId?: string
  selectedId?: string
  editing?: boolean
  onOpen(instanceId: string): void
  onPlace?(instanceId: string, x: number, y: number): void
  onStow?(instanceId: string): void
}

/**
 * 场景入口只负责选择经过校验的内容包。所有题材统一交给 ContentGardenScene，
 * 不再按植物、动物、建筑或画法建立专属渲染分支。
 */
export default function GardenScene({ specimens, packId = DEFAULT_PACK_ID, selectedId, editing, onOpen, onPlace, onStow }: GardenSceneProps) {
  const pack = getPack(packId)
  if (!pack) return <div className="garden-message" data-kind="error" role="alert">内容包 {packId} 未登记，无法展示收藏场景。</div>
  return <ContentGardenScene pack={pack} specimens={specimens} selectedId={selectedId} editing={editing} onOpen={onOpen} onPlace={onPlace} onStow={onStow} />
}

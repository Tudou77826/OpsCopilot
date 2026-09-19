import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { GardenPack } from './contentPack'
import {
  GARDEN_RUNTIME_FPS,
  advanceGardenActors,
  createGardenActors,
  gardenActorIsDynamic,
  reactGardenActor,
  type GardenActorInput,
  type GardenActorState,
} from './gardenRuntime'

interface GardenRuntimeView {
  actors: GardenActorState[]
  now: number
  reducedMotion: boolean
  running: boolean
  react(instanceId: string): void
}

function runtimeNow(): number {
  return typeof performance === 'undefined' ? Date.now() : performance.now()
}

function reducedMotionPreferred(): boolean {
  return typeof window !== 'undefined' && Boolean(window.matchMedia?.('(prefers-reduced-motion: reduce)').matches)
}

/**
 * 动态场景的唯一调度器。组件卸载、页面隐藏或减少动态效果时不会保留 RAF。
 * 运行时状态只保存在内存中，不写回成长快照。
 */
export function useGardenRuntime(pack: GardenPack, inputs: GardenActorInput[], enabled = true): GardenRuntimeView {
  const signature = useMemo(() => `${pack.id}@${pack.version}+${pack.appliedOverlays?.join(',') ?? ''}|${inputs.map(input => `${input.instanceId}:${input.itemId}:${input.slot.x}:${input.slot.y}`).join('|')}`, [pack.appliedOverlays, pack.id, pack.version, inputs])
  const signatureRef = useRef('')
  const runtimeRef = useRef<GardenActorState[]>([])
  const [renderState, setRenderState] = useState(() => ({ signature: '', actors: runtimeRef.current, now: runtimeNow() }))
  const [reducedMotion, setReducedMotion] = useState(reducedMotionPreferred)
  const [documentVisible, setDocumentVisible] = useState(() => typeof document === 'undefined' || document.visibilityState !== 'hidden')

  if (signatureRef.current !== signature) {
    const now = runtimeNow()
    signatureRef.current = signature
    runtimeRef.current = createGardenActors(pack, inputs, now)
  }
  const actorsForRender = renderState.signature === signature ? renderState.actors : runtimeRef.current
  const nowForRender = renderState.signature === signature ? renderState.now : runtimeNow()

  useEffect(() => {
    setRenderState({ signature, actors: runtimeRef.current, now: runtimeNow() })
  }, [signature])

  useEffect(() => {
    const media = window.matchMedia?.('(prefers-reduced-motion: reduce)')
    if (!media) return
    const update = () => setReducedMotion(media.matches)
    update()
    media.addEventListener?.('change', update)
    return () => media.removeEventListener?.('change', update)
  }, [])

  useEffect(() => {
    const update = () => setDocumentVisible(document.visibilityState !== 'hidden')
    document.addEventListener('visibilitychange', update)
    return () => document.removeEventListener('visibilitychange', update)
  }, [])

  const hasDynamicActor = runtimeRef.current.some(actor => gardenActorIsDynamic(actor, pack.elements[actor.itemId]))
  const animationSupported = typeof window.requestAnimationFrame === 'function' && typeof window.cancelAnimationFrame === 'function'
  const staticPresentation = reducedMotion || !animationSupported
  const running = enabled && documentVisible && !staticPresentation && hasDynamicActor

  useEffect(() => {
    if (!running) return
    let animationFrame = 0
    let lastStep = runtimeNow()
    let lastCommit = lastStep
    const frameInterval = 1000 / GARDEN_RUNTIME_FPS
    const tick = (now: number) => {
      const delta = Math.max(0, now - lastStep)
      lastStep = now
      runtimeRef.current = advanceGardenActors(runtimeRef.current, pack, now, delta)
      if (now - lastCommit >= frameInterval) {
        lastCommit = now
        setRenderState({ signature, actors: runtimeRef.current, now })
      }
      animationFrame = window.requestAnimationFrame(tick)
    }
    animationFrame = window.requestAnimationFrame(tick)
    return () => window.cancelAnimationFrame(animationFrame)
  }, [pack, running, signature])

  const react = useCallback((instanceId: string) => {
    if (staticPresentation) return
    const now = runtimeNow()
    runtimeRef.current = reactGardenActor(runtimeRef.current, pack, instanceId, now)
    setRenderState({ signature, actors: runtimeRef.current, now })
  }, [pack, signature, staticPresentation])

  return { actors: actorsForRender, now: nowForRender, reducedMotion: staticPresentation, running, react }
}

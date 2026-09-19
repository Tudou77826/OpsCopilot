import { useEffect, useRef, useState } from 'react'
import type { GardenHost } from '../ports'

const SIGNAL_POLL_MS = 5000
const ATTENTION_MS = 1400

export function observeGardenRevision(previous: number | null, next: number, isOpen: boolean): { revision: number; attention: boolean } {
  return { revision: next, attention: previous !== null && !isOpen && next > previous }
}

/**
 * 关闭养成面板时只轮询轻量 revision。首次读取建立基线，不把历史变化伪装成新提醒；
 * 后续 revision 增长时只短暂闪动入口，不生成红点、数字或待领取状态。
 */
export function useGardenAttention(host: GardenHost | undefined, isOpen: boolean): boolean {
  const [attention, setAttention] = useState(false)
  const revision = useRef<number | null>(null)

  useEffect(() => {
    if (!host) { revision.current = null; setAttention(false); return }
    let stopped = false
    let polling = false
    let attentionTimer: number | undefined

    const poll = async () => {
      if (stopped || polling) return
      polling = true
      try {
        const signal = await host.signal()
        if (stopped) return
        const next = signal?.revision ?? 0
        const observed = observeGardenRevision(revision.current, next, isOpen)
        revision.current = observed.revision
        if (!observed.attention) {
          setAttention(false)
          return
        }
        setAttention(true)
        if (attentionTimer !== undefined) window.clearTimeout(attentionTimer)
        attentionTimer = window.setTimeout(() => setAttention(false), ATTENTION_MS)
      } catch (error) {
        // 轻提示失败不影响工作流，但不能伪装成成功或切换到其他数据源。
        console.warn('养成变化信号读取失败', error)
      } finally {
        polling = false
      }
    }

    void poll()
    const interval = isOpen ? undefined : window.setInterval(() => void poll(), SIGNAL_POLL_MS)
    return () => {
      stopped = true
      if (interval !== undefined) window.clearInterval(interval)
      if (attentionTimer !== undefined) window.clearTimeout(attentionTimer)
    }
  }, [host, isOpen])

  return attention
}

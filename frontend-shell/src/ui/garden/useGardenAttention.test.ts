import { describe, expect, it } from 'vitest'
import { observeGardenRevision } from './useGardenAttention'

describe('observeGardenRevision', () => {
  it('首次读取只建立基线，不把历史变化变成提醒', () => {
    expect(observeGardenRevision(null, 8, false)).toEqual({ revision: 8, attention: false })
  })

  it('关闭时把多次变化合并为一次轻提示', () => {
    expect(observeGardenRevision(8, 12, false)).toEqual({ revision: 12, attention: true })
  })

  it('面板打开时不抢占注意力', () => {
    expect(observeGardenRevision(8, 9, true)).toEqual({ revision: 9, attention: false })
  })
})

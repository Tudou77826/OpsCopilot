/**
 * 花园场景的固定色板。
 *
 * 为什么单独成文件并登记门禁豁免：花园是收藏的一部分，必须在桌面暗色、桌面亮色和
 * Teams 三套皮肤下看起来是**同一座花园**（docs/garden-design.md §4）。因此它不引用
 * 任何工作台令牌，也不随主题变化——与 terminalSchemes.ts 是同一类"自成体系的色板"。
 *
 * 换题材（动物/建筑）时替换本文件的颜色与 packs/ 下的几何即可，状态层不受影响。
 */
export const gardenScene = {
  /** 场景底与阴影 */
  scene: '#f2ece0',
  sceneShade: '#e4dccb',
  /** 土地与围栏 */
  soil: '#d9cdb5',
  soilDark: '#463528',
  frame: '#cbb99a',
  /** 花盆/容器 */
  pot: '#8a6f57',
  /** 植株：茎、叶、花、果 */
  stem: '#4a7c3f',
  stemYoung: '#79a86a',
  leaf: '#5f9e4f',
  leafDeep: '#3f7a3a',
  bloom: '#d98cb3',
  bloomCore: '#f2d06b',
  fruit: '#c4553f',
  guardPetal: '#8f7fd4',
  transferFrond: '#63b0a4',
  knowledgeCanopy: '#6f9e4a',
  /** 闪光 */
  shinyGlow: '#ffe9a3',
  shinyMark: '#c9a227',
} as const

/** 面板外的界面色（卡片、文字、进度），同样固定。 */
export const gardenPanel = {
  card: '#fffdf7',
  text: '#2f2a22',
  textMuted: '#6b6154',
  accent: '#6f9e4a',
  danger: '#a12622',
} as const

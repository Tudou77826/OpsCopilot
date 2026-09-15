/**
 * 花园植株的固定美术色板。
 *
 * 为什么单独成文件并登记门禁豁免：花园是收藏的一部分，三套皮肤下必须是
 * 同一批植株（docs/garden-design.md §4）。植株本体不随主题变化；昼夜差异
 * 只发生在场景层（天空/地面/月光滤镜，见 gardenScene.css 的 --g-* 变量）。
 *
 * 换题材（动物/建筑）时替换本文件的颜色与 packs/ 下的几何即可，状态层不受影响。
 */
export const gardenScene = {
  /** 花盆/容器（详情抽屉的 potted 形态） */
  pot: '#8a6f57',
  soilDark: '#463528',
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
  /** 场景杂件：野花的三种花色与花芯 */
  wildflowers: ['#ffd1e8', '#fff3b0', '#e8d5ff'],
  wildflowerCore: '#e8871e',
} as const

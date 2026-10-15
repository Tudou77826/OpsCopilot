# 功能奖章素材

`source.png` 是使用内置 ImageGen 生成的八列四行透明母版，完整提示词保存在 `prompt.txt`。列顺序为知识归档、脚本创建、快捷命令创建、案例完成、知识阅读、脚本使用、快捷命令使用、活跃日；行顺序为未解锁、铜、银、金。

正式图标为 128 × 128 全色透明 PNG，保留金属高光、珐琅底色与半透明边缘。生成工具只分割、等比缩放和无损编码，不进行索引色压缩或二次像素化。使用共同缩放比例，保留至少四像素可见轮廓边距。

```powershell
node tools/pixel/build-medals.mjs art/companion/medals-v2/source.png frontend-shell/src/companion/medals/v2
node tools/checks/check-companion-assets.mjs
node --test tools/checks/check-medal-assets.test.mjs
```

奖章资源使用独立清单，不改变成长阶段与角色动作契约。门禁要求八类四等级完整、路径安全、尺寸正确、轮廓不裁切、图像不重复；单图上限 70 KB，整套上限 1.5 MB。展示不进行换色或 Canvas 重绘，也没有常驻动画。名称、等级、数量和可选状态由应用数据决定，素材不包含业务规则。

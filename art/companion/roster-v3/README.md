# 多种待机与走路素材

七种角色各有一张由内置 ImageGen 编辑生成的透明母版，参考 `roster-v2` 保持身份与画风。完整提示词见 `prompts.json`。八列四行分别为眨眼参考、环顾、舒展、向右迈步；走路左向由同一组姿态镜像显示。

正式包使用 `packs/<species>-v5`。原有互动动作保留；常规待机以站立首帧为参考，固定脚底，并通过下半身图像配准消除横向偏移。新增环顾、舒展同样固定脚底；走路保留真实迈步姿态，由展示组件控制场景内移动。

```powershell
node tools/pixel/build-roster-ambient.mjs
node tools/checks/check-companion-assets.mjs
node --test tools/checks/check-ambient-assets.test.mjs tools/pixel/*.test.mjs
```

工具根据透明间隔调整母版分割线，避免原稿行列留白不均造成邻帧脚部混入；保留主体附近装饰、软边缘与完整色彩。校准报告见 `registration.json`。生成结果保持 256 像素 RGBA，无颜色量化或二次像素化。每角色 56 张，共用三阶段比例；资源只有外观与播放节奏，不含经验、排名或执行规则。

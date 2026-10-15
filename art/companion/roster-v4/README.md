# 独立动作原稿与正式 v6 包

七种伙伴各七组独立 AI 动作原稿：待机、招呼、开心、休息、环顾、舒展和行走。原稿由内置 ImageGen 生成，保留完整 RGBA 与透明背景；`*-candidate-*.png` 是被替换的候选，不参与构建。

`prompts.json` 保存公共动作提示词、物种与参考包；最终微光和浪豆修正追加规格在 `corrections.json`。参考来自 `<species>-v5` 的站姿，行走另参考右向姿态。微光开心最终版本追加要求：完整展开的耳朵位于每格中央50%，每侧25%透明留白，并保留两个头顶金色星簇。

| 原稿动作 | 网格 | 输出 |
| --- | --- | --- |
| idle、sleep | 4×2 | 各8帧 |
| greet、happy、look、stretch | 4×3 | 各12帧 |
| walk | 8×4 | 32帧：0–3转出，4–7起步，8–23循环，24–27停步，28–31转入 |

```powershell
node tools/pixel/build-roster-actions.mjs
node tools/checks/check-companion-assets.mjs
```

生成过程在 `staging/<species>` 完成七包验证后，才写入 `frontend-shell/src/companion/packs/<species>-v6`。`build-report.json` 保存每包缩放与字节数。运行登记见 `frontend-shell/src/companion/rosterAssets.ts`。

正式帧按原稿校准并添加共享端点，输出256×256全色透明PNG。基础动作12fps，步行循环16fps；帧数、真实姿态变化和播放速度分别验证。待机保留长站立，不持续快速播放。完整处理和验收规则见 [生产规范](../../../docs/companion-art-pipeline.md)。

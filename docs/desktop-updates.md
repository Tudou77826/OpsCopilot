# Windows 桌面交付与升级

## 使用方式

统一交付 `opscopilot-windows.zip`。解压整个包，运行 `opscopilot.exe`。程序自带 JDBC helper 和 Java 21，不需要用户安装 Java；数据库厂商驱动由用户导入自己的 JAR。

程序首次启动会在后台校验并准备数据库组件。准备期间其他功能可以使用，数据库连接需等待组件就绪。失败时显示原因并提供重试，也可修复安装包后重新启动。组件安装不需要联网。

桌面的“检查更新”下载完整包并重启程序；重启后组件就绪才视为整个更新完成。用户连接、凭据、策略、知识库和导入驱动属于用户数据，不随安装包替换。

## 包结构与兼容方式

压缩包根目录只有：

```text
opscopilot.exe
jdbc-runtime.bundle
```

`jdbc-runtime.bundle` 是包含 helper、私有 Java 及许可证的压缩载荷。主程序编译时绑定其 SHA-256。旧更新器只复制根目录文件，也能送达这两个文件；新程序启动后完成组件安装。因此以后每个版本都保留首次启动安装能力，用户可以跨版本升级，无需先安装一个过渡版。

组件安装到 `jdbc-runtime/managed/<SHA-256>/`。先在同级临时目录解压，检查文件完整性，实际启动 Java 21 和 helper，再一次性提交目录。主程序只选择与自身哈希匹配且本次启动校验通过的版本。已有旧版 `jdbc-runtime` 文件和已提交版本保留，组件失败不会覆盖原有文件。

新更新器在文件复制失败时恢复原程序并移除本次引入的残缺文件。组件准备失败时保留更新备份和结果，禁止使用不匹配组件，允许重试；不会自动切回一个不匹配的 helper。旧更新器本身的错误处理无法远程修复；若旧程序在复制阶段失败、无法启动，可退出程序后将完整包重新解压到原目录恢复，保留用户数据。禁止在程序或 Java 仍运行时手动覆盖组件。

当前保留历史组件供恢复使用，不自动删除旧组件版本。升级需预留载荷、解压组件及旧版本同时存在的空间。

## 构建与验证

构建机需要 Go、Node.js、Wails、Maven 和 Windows x64 Java 21 JDK。运行：

```powershell
./scripts/build-desktop.ps1 -Version v1.2.3 -JdkHome 'D:/tools/jdk-21'
```

`build_release.bat`、`build_release_jdbc.bat` 也调用同一构建流程。输出目录必须不存在，每次构建保留独立产物。包内容使用显式清单，不复制开发机的配置、账号和驱动。输出包括 `opscopilot-windows.zip` 和 `SHA256SUMS.txt`。GitHub 标签发布使用相同脚本；内网发布仍上传同名标准 ZIP。

本地升级实测：

```powershell
./e2e/harness/desktop-update.ps1 -Package '完整包路径' -LegacyExecutable '旧版程序路径'
```

该验证在独立目录运行旧程序的 `--self-update`，覆盖全新启动、已有／没有旧 JDBC、损坏载荷和恢复重试。记录保存为 `results.json`；仅关闭测试目录中的程序，不操作正在使用的预览。

本机制不更新用户的厂商 JAR，不自动同步外部 Skill，也不负责服务器 CLI 升级。

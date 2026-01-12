# Markdown渲染功能使用说明

## 功能概述
AI问答面板现在支持完整的Markdown格式渲染,包括:
- ✅ 标题 (h1-h6)
- ✅ 粗体、斜体、删除线
- ✅ 代码块(带语法高亮)
- ✅ 行内代码
- ✅ 列表(有序、无序)
- ✅ 任务列表
- ✅ 表格
- ✅ 引用块
- ✅ 链接(自动在新窗口打开)
- ✅ 分割线
- ✅ GFM扩展语法

## 测试方法

### 1. 启动应用
```bash
# 开发模式
npm run dev

# 或构建并运行
npm run build
# 然后在根目录运行wails应用
```

### 2. 测试Markdown渲染

在AI问答输入框中输入以下内容进行测试:

#### 测试1: 基础格式
```
请用Markdown格式回复我,包含**粗体**、*斜体*和`代码`
```

#### 测试2: 代码块
```
请给我一个JavaScript函数示例,用代码块展示
```

AI可能返回类似:
````markdown
这是一个示例函数:

```javascript
function greet(name) {
    console.log(`Hello, ${name}!`);
    return true;
}
```
````

#### 测试3: 列表和表格
```
用Markdown表格对比Python和JavaScript的特点
```

AI可能返回:
```markdown
| 特性 | Python | JavaScript |
|------|--------|------------|
| 类型 | 强类型 | 弱类型 |
| 用途 | 数据科学、Web | Web前后端 |
| 语法 | 简洁 | 灵活 |
```

#### 测试4: 完整示例
```
用Markdown格式介绍Git的基本命令
```

预期返回类似:
```markdown
# Git基本命令

## 配置
- `git config` - 配置用户信息
- `git init` - 初始化仓库

## 基本操作
1. **添加文件**: `git add <file>`
2. **提交更改**: `git commit -m "message"`
3. **查看状态**: `git status`

> 提示: 经常使用 `git status` 查看当前状态
```

## 实现细节

### 核心组件
- **MessageRenderer.tsx**: Markdown渲染组件
  - 使用 `react-markdown` 解析Markdown
  - 使用 `remark-gfm` 支持GitHub风格
  - 使用 `rehype-highlight` + `highlight.js` 实现语法高亮

### 样式特性
- 🎨 暗色主题适配(Atom One Dark代码高亮)
- 📱 响应式设计
- 🔗 外部链接安全打开(target="_blank", rel="noopener noreferrer")
- 📊 表格横向滚动
- 💻 代码块带语言标识

### 支持的代码高亮语言
包括但不限于:
- JavaScript/TypeScript
- Python
- Java
- C/C++
- Go
- Rust
- HTML/CSS
- SQL
- Bash/Shell
- JSON/YAML
- Markdown
- 等等...

## 文件结构

```
frontend/src/components/Sidebar/
├── AIChatPanel.tsx          # AI问答主面板(已更新)
├── MessageRenderer.tsx      # Markdown渲染组件(新建)
└── MessageRenderer.test.tsx # 单元测试(新建)
```

## 技术栈

### 依赖包
```json
{
  "react-markdown": "^10.1.0",      // Markdown渲染
  "remark-gfm": "^4.0.1",            // GFM支持
  "rehype-highlight": "^7.0.2",     // 代码高亮
  "highlight.js": "^11.11.1"        // 语法高亮核心
}
```

### 开发依赖更新
- `vite`: ^3.0.7 → ^5.4.21 (解决兼容性问题)
- `@vitejs/plugin-react`: ^2.0.1 → ^4.x

## 性能优化

1. **组件优化**
   - 使用 `React.memo` 避免不必要的重渲染

2. **样式内联**
   - 样式直接内嵌在组件中,避免额外的CSS文件

3. **按需高亮**
   - highlight.js自动检测语言,只加载需要的语言支持

## 已知限制

1. 不支持LaTeX数学公式(可通过添加remark-math和rehype-katex支持)
2. 不支持原始HTML(安全考虑,可通过rehype-raw启用)
3. 代码块最大宽度受消息气泡限制(可横向滚动)

## 未来增强

- [ ] 代码块复制按钮
- [ ] 图片点击放大
- [ ] LaTeX数学公式支持
- [ ] Mermaid图表支持
- [ ] 代码行号显示
- [ ] 主题切换(多种代码高亮主题)

## 故障排查

### 问题1: Markdown不渲染
**检查**: 确保已安装依赖
```bash
cd frontend
npm install
```

### 问题2: 代码高亮不显示
**检查**: highlight.js样式是否正确导入
- 文件: `MessageRenderer.tsx`
- 导入: `import 'highlight.js/styles/atom-one-dark.css';`

### 问题3: 构建失败
**解决**: 确保Vite版本≥5.0
```bash
npm install vite@^5 @vitejs/plugin-react@^4 --save-dev
```

## 参考资源

- [react-markdown文档](https://github.com/remarkjs/react-markdown)
- [remark-gfm](https://github.com/remarkjs/remark-gfm)
- [highlight.js](https://highlightjs.org/)
- [GitHub Flavored Markdown规范](https://github.github.com/gfm/)

---

**版本**: 1.0.0  
**最后更新**: 2026-01-12  
**维护者**: OpsCopilot团队

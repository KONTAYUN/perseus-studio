# 文稿编辑器成熟方案调研 — 2026-10-23

## 建议

保留 Perseus 的 Markdown 工作区、文件服务、Agent 工具及保存冲突机制。先补齐用户可用的恢复与编辑保真保护，再用独立样机验证 Vditor，Milkdown 作为备选。不要仅凭“成熟项目”或演示页直接替换生产编辑器；本轮完成的是官方资料和部分实现对照，没有运行候选编辑器的同语料对比实验。

当前 Tiptap 富文本内核的可用性，不能直接推导出其 Markdown 双向转换的完整性。官方 Markdown 文档明确标记 Beta/early release，列出转换限制。我们的 `RichTextEditor.onUpdate` 仍通过 `editor.getMarkdown()` 重新生成当前文档或分段，即使用户只改了一小部分；局部原文保留解决了已识别内容的覆盖风险，不能证明未知结构全部可无损转换。

## 可借鉴项目与机制

| 项目/资料 | 官方资料确认的机制 | 对 Perseus 的价值和限制 |
| --- | --- | --- |
| Vditor | 浏览器 Markdown 编辑器，所见即所得/即时渲染/分屏三种模式；CommonMark/GFM、脚注、公式、YAML 头信息等支持 | 第一候选样机，常见扩展已有成套能力；仍需验证未知扩展、安全策略、原文保真、React 集成、表格/输入法/撤销及 Agent 外部刷新。不能假设三模式切换无损 |
| Milkdown | ProseMirror + remark，语法与编辑器模型由插件映射 | 适合精细掌控语法插件与块交互；源码中缺少匹配 schema 的 Markdown 节点会抛错，不能视作默认支持任意未知语法。迁移后仍需显式保留策略 |
| TOAST UI Editor | CommonMark/GFM、自定义块，富文本下先呈现自定义块，再点击局部编辑 | 证明“复杂内容局部编辑”是可借鉴的成熟交互；其官方文档同样说明任意 HTML 不会自动获得完整富文本支持，须配置映射 |
| CodeMirror 6 | 原文作为编辑文档，装饰/替换区域和 widgets 改变视觉，事务映射维护位置 | 适合长期探索原文保留的即时预览；装饰本身不要求重写源文。完整面向非技术用户的表格、选区工具栏和编辑语义仍需要大量产品开发 |
| CKEditor 5 的 Markdown 输出说明 | 正式列出 Markdown 不能承载的高级列表样式、表格样式、分页等信息会在输出时被去除 | 产品应声明支持的文稿格式，不能把“富文本能显示”当作“MD 能保存” |
| Obsidian 文件恢复说明 | 定期完整快照、查看差异、复制或恢复；恢复快照不等于完整备份 | 借鉴用户可发现的恢复交互。Obsidian 不是本轮可替换的开源编辑器内核，也不作为其内部实现的公开证据 |

## 结合当前代码的落实顺序

1. **先让已有版本真正可恢复。** 后端 `workspace/file_write.go` 已在覆盖前保存 `file_versions`，快照写入失败会中止覆盖；已有原子文件写和预期哈希冲突检测。本轮代码搜索未发现通用的文稿版本列表/预览/恢复界面，现有工具变更撤销不能代替该入口。应复用现有存储，补齐按用户/文件查询、预览差异、受哈希保护的恢复、恢复前留当前版本、重命名身份与保留策略，不另造平行版本系统。
2. **明确一种主数据。** 当前 Agent 与工作区工具读写 Markdown，优先保留 Markdown 为权威文件。编辑器 JSON 可以作为缓存，不能在缺少同步契约时与 Markdown 同时成为真源。若未来改为结构化文稿存储，应独立立项改造 Agent 编辑协议和导入导出，不能局部偷换。
3. **逐步减少编辑范围之外的重写。** 利用语法树位置和编辑事务追踪被修改的结构，未修改范围拼回原文；列表、表格、引用、引用定义等影响周围语法的修改需要扩大到闭合结构，不能简单按视觉行截取。整段重新生成仍有必要时，保留原稿与编辑草稿，不假设转换自动保真。
4. **以常见语法的原生交互替代频繁兜底。** 表格、图片、脚注、公式优先做可视化支持；少见自定义结构保留原始载荷，正文可编辑。无需用户理解 Markdown 模式，也无需 Agent 才能恢复正常编辑。安全的文字/编辑展示可以保留，原始脚本和危险属性不能执行。
5. **保存保护不能只看关键字、字符数量或一次重解析。** 检查未编辑范围是否逐字一致、保留节点载荷是否保持、事务是否影响了声明范围之外的结构。解析后再序列化成功并不能证明语义正确。无法证明安全时先保存独立草稿并保留正式原稿，给出可操作的恢复入口，避免锁住编辑器或反复弹窗。
6. **统一人和 Agent 的恢复边界。** 人工保存、Agent 写入、外部刷新和冲突处理共同遵守文件身份、快照和预期哈希；不能由旧编辑缓存覆盖 Agent 新版本，也不能让恢复操作无条件覆盖他人刚写的内容。

## 候选替换实验与验收

候选只接入隔离样机，沿用现有编辑组件的 `content/onChange` 适配界面及最终文件保存边界。单纯包装 `getMarkdown()` 不等于完成保真保护。常见格式与原文保护分别验收。

- 使用匿名化真实文稿与现有回归语料，覆盖嵌套列表、引用、表格换行/管道、图片、长链接、脚注/引用定义、公式、HTML、容器、Wiki、BOM/CRLF、缺失闭合、长文档和未知扩展。
- 分别测试打开不写入、零编辑切换后原文不变、只改正文后未编辑范围保持、未知载荷保持、连续多轮保存/刷新无累积退化。
- 实际执行输入法组合输入、复制粘贴、选区/段落格式、增删表格行列、撤销重做、节点删除/退出、窄屏与深浅主题，不能只做解析器单测。
- 验证保存失败、本地草稿、外部 Agent 更新、断网重连、冲突及历史恢复后的真实磁盘内容。所有写入性测试使用独立数据库与用户目录。
- 为安全损坏案例设硬门槛：不可出现静默丢字、丢未知节点、编辑范围外的意外覆盖或不可恢复覆盖。其余允许的 Markdown 格式规范化应明确记录，不能冒充逐字无损。

只有同一语料下通过上述门槛，才能决定是否替换现有内核；此时保留原有工作台 UI、工具和文件服务，通过适配器渐进切换。维护活跃度、发布版本、依赖与许可证也需要在选定版本时复核，不能仅凭 README 判断。

## 一手来源

- Tiptap Markdown：<https://tiptap.dev/docs/editor/markdown>；文档源码：<https://github.com/ueberdosis/tiptap-docs/blob/main/src/content/editor/markdown/index.mdx>
- Tiptap 持久化建议：<https://tiptap.dev/docs/editor/core-concepts/persistence>
- ProseMirror Markdown 示例：<https://prosemirror.net/examples/markdown/>
- Vditor：<https://github.com/Vanessa219/vditor>
- Milkdown：<https://github.com/Milkdown/milkdown>；解析器：<https://github.com/Milkdown/milkdown/blob/main/packages/transformer/src/parser/state.ts>
- TOAST UI 自定义块与 HTML 节点：<https://github.com/nhn/tui.editor/blob/master/docs/en/custom-block.md>
- CodeMirror 装饰示例：<https://codemirror.net/examples/decoration/>
- CKEditor Markdown 输出与已知限制：<https://ckeditor.com/docs/ckeditor5/latest/features/markdown.html>
- Obsidian 文件恢复公开文档：<https://help.obsidian.md/plugins/file-recovery>；本次读取正文来源：<https://github.com/obsidianmd/obsidian-help/blob/master/en/Plugins/File%20recovery.md>

获取的一手页面和源码副本保存在忽略目录 `_local_artifacts/editor-research-20261023`。Tiptap 独立 `getting-started/limitations` 路径为 404，本报告使用有效的 Markdown 总览正文；部分文档网站为客户端加载，已改查其公开 Markdown/源码，不将 HTTP 200 空壳页面当作正文证据。

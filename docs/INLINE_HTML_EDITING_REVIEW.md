# 行内 HTML 与富文本编辑兼容性

2026-10-20。用户截图中的只读提示来自富文本兼容性保护，并非文件损坏或权限问题。

## 根因与修改

实际文档 `v18.2.md` 第 72 行使用 `<span style="…">` 包裹行内代码；旧规则遇到任何 HTML 就对整篇启用只读预览。没有直接取消保护，因为默认编辑器会丢掉不认识的结构或属性。

| Before | After | Why |
| --- | --- | --- |
| 一处行内颜色标签使整篇文档只读 | 平衡、单行的 span 由独立行内容器承载，正文和其中的代码、加粗、链接可编辑 | 缩小兼容性限制，不牺牲原始标签 |
| HTML 只有预览/源码入口 | 起止标签原样存为节点属性；保存时重新输出，DOM 仅使用编辑器自己生成的中性 span | 不执行原文样式、事件或脚本；避免导入的固定黑白色破坏深色模式 |
| 对嵌套与代码中的伪标签缺少契约 | 共享识别器处理嵌套、带引号的 `>`、代码内伪标签，限制嵌套深度，未闭合/跨行保留只读保护 | 兼容性检查与编辑器解析共用边界 |

仍不支持任意 HTML、跨行 span、frontmatter、脚注、数学等扩展的直接富文本编辑；这些仍可通过纯文本编辑。span 的原始颜色属性被保留但不在富文本里执行，要调整原始样式请切源码。Markdown 保存会规范化列表符号、空白与实体写法，不保证逐字节排版不变；打开/切换模式必须保持原文且不能写盘。

## 验证

- 149/149 单测通过；新增 span 标签、嵌套、代码、链接、引号、空 span 和不支持结构的回归。
- Markdown 浏览器专项 7/7；首次全套发现测试中通过点击加 End 定位光标不稳定（文字插入到了代码中间，标签未丢失），改为明确选择和折叠选区后专项连续 3/3 通过。
- 实际文档复制到独立测试库后，完成富文本编辑/保存；5 个代码块内容、原始 span 标签保持，打开与切换 0 写入，0 页面异常。没有修改用户原件。
- UI/结构门禁通过（135 个前端文件、279 个 Go 文件）；生产构建通过，保留既有大 chunk 提醒。新增直接声明已安装的 `@tiptap/core`，未增加另一套编辑器。

日志位于 `_local_artifacts/inline-span-unit.log`、`inline-span-build.log`、`inline-span-e2e.log`、`inline-span-e2e-repeat.log`、`inline-span-real-document.log`。全套最终结果与手动更新记录见下方交付记录。本轮不涉及后端变更，未重复执行 Go/容器测试或调用真实模型。

## 交付记录

- 最终 Chrome 全套 **26 passed / 1 live-provider opt-in skipped**（53 秒），`inline-span-e2e-all-final.log`。没有用提高重试次数掩盖首次定位不稳定的问题。
- 14:26 更新本地 8080。切换前复核调度运行/排队与持久活动任务均为 0，用户文件与业务表摘要仍匹配本轮基线；停止旧进程后完整离线备份至 `_local_artifacts/manual-backups/20261020-inline-span/`。
- 后端仍为 `_local_artifacts/perseus-handoff-20261020.exe`，前端改用 `_local_artifacts/frontend-inline-span-20261020/`，新目录保留旧 assets。PID `170576`、exec session `28628`，原数据库与用户目录沿用。
- 原账号登录并打开实际 `v18.2.md`，确认可进入富文本、模式切换原文一致、0 写入、0 页面异常；原 3 个用户文件、用户/模型摘要和 9 张业务表摘要一致，SQLite 检查及 ready 正常。证据 `inline-span-manual-document.log` 与本轮 `pre-inline-span-*.json` 基线。
- 独立测试实例结束后停止，测试数据与日志保留。未推送、未远端部署、未发送外部消息。

后续 22:42 后台 patch 恢复修复：8080 后端已更新为 `_local_artifacts/perseus-patch-recovery-20261020-final.exe`，前端仍沿用本报告目录。最新 PID/备份/验证和重启组合见 [PATCH_RECOVERY_REVIEW.md](PATCH_RECOVERY_REVIEW.md)。

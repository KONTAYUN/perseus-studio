# 设置页面外层方框修复

2026-10-23。针对用户提供的 Skills 与用户管理截图，排查所有设置分类的外层容器。

## 原因及修改

设置重构后，`SettingsSection` 仍带旧卡片的边框、背景、内边距，同时 CSS 中保留圆角和内阴影。新页面用另一条选择器覆盖外观，深色内阴影规则的优先级更高，留下方形细框。浏览器在修复前实际读到外层容器 `box-shadow: rgba(255, 255, 255, 0.043) 0px 0px 0px 1px inset`，回归失败记录见 `before.log` 和 `before-failure/`。

| Before | After | Why |
| --- | --- | --- |
| 共用设置区块默认是一张卡片，新页面再覆盖其样式 | 共用区块作为透明、无边框的页面容器 | 从源头消除外框，避免浅深色级联冲突 |
| 旧圆角、内阴影和新页面覆盖规则同时存在 | 删除区块的旧装饰和补丁式覆盖 | 不靠增加优先级或 `!important` 隐藏问题 |
| 整页容器与内部卡片都描边 | 保留内部卡片、空状态、表单和键盘焦点的必要边界 | 保持可辨认的层次和可访问性 |

源代码修改仅涉及 `src/components/ui/LayoutPrimitives.jsx` 和 `src/styles/settings.css`，没有更改业务接口和数据结构。

## 验证

全部故障注入、浏览器操作使用 18089 独立数据库与 Playwright 隔离浏览器上下文，没有登录或修改手动实例账号，没有请求真实模型。

- 新增 `tests/e2e/settings-surfaces.spec.js`，4 项通过：浅深色各 6 分类 × 桌面/390px，外层四边、阴影和背景实际计算值；内部卡片和键盘焦点；用户加载与创建；已安装扩展及其详情。
- 现有设置工作区 11 项全部通过，包含草稿保护、提交/刷新失败、菜单、价格保存，以及切换设置期间实际后端 Agent 的结果接收。合计 **15 项通过**，`browser.log`。
- 构建、UI 和结构门禁通过。没有重跑全项目 Go/单元/完整浏览器套件，本轮没有后端变化。构建仍有既有的 >500kB 分块提示。
- 人工查看深色 Skills、用户管理桌面和窄屏截图，确认截图中的额外方框消失。浅深色各页均由浏览器断言核对；不等同于全站或所有浏览器的视觉审计。

证据目录：`_local_artifacts/settings-surfaces/`，包括 `skills-dark-1440.png`、`users-dark-1440.png`、`users-dark-390.png` 和 `skill-detail-dark.png`。

## 手动实例

确认没有运行/排队 Agent、待执行任务或待提交消息，记录新基线，停止原进程后完成离线备份，再启动修复版。未修改用户凭据与原数据。

- 地址：`http://127.0.0.1:8080/`，本次启动 PID 128848。
- 仍用原后端 `_local_artifacts/perseus-settings-refactor.exe` 和原数据 `_local_artifacts/manual-runtime/`。
- 新前端：`_local_artifacts/frontend-settings-surfaces/`。
- 备份：`_local_artifacts/manual-backups/20261023-settings-surfaces/`。
- 进程与交接：`_local_artifacts/settings-surfaces/manual-process.json`、`manual-integrity.json`、`delivery-verification.json`。
- SQLite 完整性、外键和 readiness 通过。83 个原文件、26 张表的 5467 条原记录哈希保持，受保护业务表数量一致。8080 提供的 6 个静态资源均与新构建逐字节相同。旧构建与旧备份保留。

刷新浏览器后加载新版本。之前设置重构交付报告中的 PID、前端目录与测试统计属于上一版，本次交接以本文件为准。

# Perseus 全功能与 Agent 可靠性复验记录

批次 `20261023-1215`，2026-10-23 12:15 开始，13:54 收尾。按 [批准计划](AGENT_FULL_FUNCTION_ACCEPTANCE_PLAN.md) 执行；以下记录为当时冻结构建的结果。用户随后批准修复 AC-01～AC-03，16:25 更新手动实例，修复后证据另见 [验收后可靠性修复](ACCEPTANCE_RELIABILITY_FIXES.md)，不覆盖本次原始现场。

证据目录：`_local_artifacts/agent-full-function/20261023-1215/`。

## 基线

8080 PID 128848，启动时无运行/排队 Agent、待执行后台任务或待发输入。已在线一致性备份数据库，并在空闲时复制用户目录；原 83 文件、26 张表原行摘要及工作树差异已保存。未重启或修改 8080 配置。

## 中途结论（13:08，长时测试尚未结束）

本轮没有修改产品源码、替换 8080 构建或调整原配置。基础回归和真实文稿主流程已完成；90 分钟混合运行仍在进行，最终结论待其结束及最后一次完整性核对。不能据此宣称“全功能无问题”。

### 已确认的平台问题

1. **AC-01 / P2：编辑契约说明互相矛盾，待修复。** 实际执行器只执行 JSON `apply_workspace_patch`，正文和代码块只展示。可实际发送给模型的 `tool_schemas_workspace.go:213` 仍声称完整正文补丁也受支持；`tasks/manager_ai_helpers.go` 的 executionPrompt、repairPrompt 同样要求直接输出补丁并称系统会执行。隔离供应商按照正文方式输出后没有生成文件；这符合当前执行边界，却违背上述说明。真实 Grok 使用 JSON 工具时成功，不能证明矛盾没有风险。应统一 schema、前台/后台/修复提示为结构化工具，增加出站契约一致性及“按说明调用可真实写入”的回归，不能恢复正文执行来绕过问题。证据：`deep-wire-followup.log`、`deep-wire-followup-captures.json`、`task-pagination.json`、`model-wire.json`。
2. **AC-02 / P3：后台失败诊断缺少模型归属。** 校验耗尽的诊断卡显示“未记录模型”，任务本身有 provider_id；`failAIAgentTask` 没有传入供应商/模型上下文。task/run 仍可追踪，属于诊断完善项。证据：`repair-exhaustion-followup.json`、`settings-zoom-CDP.png`。
3. **AC-03 / P3：未执行的命令仍带“已运行命令”标签。** 禁用命令在参数校验阶段拒绝，status=failed、verification=skipped，且没有进入工具执行账本，保护本身有效；通用 label 却沿用完成式措辞，容易误解。应按拒绝/失败/成功生成状态说明。证据：`command-bypass-followup.json`。

### 已执行及证据层级

| 检查 | 本轮实际结果与范围 | 证据（均在本批次私有目录） |
| --- | --- | --- |
| 构建身份 | 当前产品源码与 8080 构建对应，前端资源摘要一致；保留原脏工作树 | `build-identity.json`、`tracked-before.patch`、`worktree-before.txt` |
| 前端 | 157 单测、UI/结构门禁、生产构建通过 | `frontend-unit.log`、`ui-standards.log`、`structure-standards.log`、`frontend-build.log` |
| 后端 | uncached Go 全包、vet、Windows 构建通过；Unix 权限位测试在 Windows 跳过 | `go-test-uncached.log`、`go-vet.log`、`backend-build.log` |
| Chrome 全套 | 55 通过，1 个真实供应商 opt-in 跳过；套件中有明确 UI 接口拦截的时序用例，不全部宣称实际执行器端到端 | `browser-all.log`、`browser-results/` |
| 隔离实际 HTTP | 19 个 API 验证通过；身份/角色/密码/隔离、配置脱敏、文件/回收站、幂等、附件下一轮、取消 | `isolated-api-results.json` |
| 真实 8080 文稿流程 | 20 个 run、2 个后台任务；读材料→提问→六稿→精确修订→排队/steer→取消/继续→编辑冲突→格式修复→压缩/分支→后台审校→公开联网取材 | `live-01.log`～`live-07.log`、`live-evidence.json`、`runs/` |
| 真实搜索 | Tavily 搜索与 CommonMark 正文提取成功；20k 截断如实呈现，结果真实写入并回读。只发公开问题，没有将私有文稿作为查询 | `search-live.json` |
| 执行与调度 | 401 一次、429/502 各最多三次并退避；重复读取有界停止；同用户串行、跨用户并发、队列 8 受理及溢出 503、排队取消零请求 | `runtime-extra.log`、`queue-followup.log` |
| 实际工具/事件 | 旧补丁拒绝、人工版本保留；steer 在在途旧响应之后的两次请求保持；自由答复实际送达；正文补丁不执行；命令绕过拒绝 | `deep-wire.log`、`deep-wire-followup.log`、`command-bypass-followup.json` |
| 后台修复与分页 | 缺产物下一轮补做成功；持续缺失在初次+三次修复后明确失败；214 持久事件三页完整读回 | `task-pagination.log`、`repair-exhaustion-followup.json` |
| SSE | 两个独立浏览器上下文、断网/刷新重连、270 条持久事件、119 游标重放、14 次读取，唯一终态且无重复写入 | `dual-stream-verified.log`、`dual-stream.json` |
| 真正进程崩溃 | 隔离前台和后台实际写后强杀；前台显式续跑不重复首个补丁；后台明确 recovery_required，不自动重放 | `recovery.log`、`recovery-results.json` |
| 匹配恢复副本 | 库/文件恢复到新目录；登录、新任务及实际 UI 撤销/刷新/禁止二次撤销通过；SQLite 检查通过 | `recovery-followup-verified.log`、`recovery-followup.json` |
| 转交持久化故障 | 独立库 trigger 拒绝前台 run_completed：转交事务回滚、任务 0；移除 trigger 后显式恢复同一 run，单任务/单补丁完成 | `handoff-fault.log`、`handoff-fault.json` |
| 参数链路 | UI 设置 120000 tokens/85% 保存重开，深思考实际请求 reasoning_effort=high | `model-wire.log`、`model-wire.json` |
| Skills | 自写无外部动作 zip：安装/读取/改名/启禁用/可见性/删除、多候选选择；非法 zip 400、27 MiB 413 | `skills-verified.log` |
| 编辑竞态 | A/B 读取乱序不串稿；脏标签确认取消保持；显式保存关闭不丢；格式修复等待保存时切会话不误发 | `editor-races-followup.log`、`editor-target-race-followup.log` |
| 容量 | 50 文件、5000 段约 159k 字符富文本编辑，300 数据行表格最后格编辑，600 KiB 源码降级；内容核对通过 | `performance-ui.json`、`performance-ui-followup.log`、`table-ui-followup.log` |
| 缩放与键盘 | 实际 Chrome 默认缩放 200%，DPR=2，窗口 1560/内容 772；六分类键盘进入及返回焦点通过，无水平溢出 | `zoom-keyboard-followup.log`、`zoom-bounds.json`、`settings-zoom-CDP.png` |
| 滚动与回收站 | 130 段实际 SSE 中上翻不抢滚动、点到底部恢复跟随；隔离用户永久删除/清空均测取消和确认，其他用户不变 | `scroll-stream.log`、`trash-ui-followup.log` |
| 中途原数据 | 83 个原文件哈希、26 表原行全部保持；配置/构建不变，quick_check=ok、外键错误 0、活动/待发均 0；用量记录增量 108 | `integrity-interim.json` |

构建仍有既有大 chunk 提醒（主包约 660 kB、富文本包约 554 kB），不是运行失败；没有借此宣称加载性能已优化。

## 最终收尾（2026-10-23 13:54，北京时间）

- 90 分钟 soak 从 12:20:20 持续到 13:50:20，实际运行 5400.317 秒；完成 91 个隔离短任务、182 个浏览器任务轮次、1336 次 HTTP 请求，服务始终 ready/accepting，结束时 running=0、queued=0。浏览器/页面错误均为空。完整结果见 `soak-result.json`、`soak-browser-errors.json`。
- 同一时间窗口的编辑器伴随测试完成 68 轮（源码/富文本交替、保存、任务面板打开关闭），0 页面错误；`soak-editor-companion-result.json`。这证明长时间混合操作没有观察到崩溃或明显的编辑器串稿，但不是内存泄漏的形式化证明。
- 资源首样本到末样本：工作集约 37.5→78.2 MiB，私有内存约 54.4→63.7 MiB，句柄 232→290；前几分钟完成浏览器/数据库热身，后段在约 63–68 MiB 私有内存、约 290 个句柄附近波动。数据库 432 KiB→21.4 MiB 与 1336 次持久请求/事件增长一致；没有队列积压或句柄持续失控证据。采样原文在 `soak-samples.jsonl`。
- 最终 8080 空闲核对：原进程仍在，隔离 18090–18093 已在测试完成且空闲后关闭。最终一致性核对通过：83 个基线文件无变化、所有基线业务表原行均存在且未变、二进制摘要匹配、SQLite `quick_check=ok`、外键错误 0、running/queued/pending 均为 0。最终用量增量为 108。证据 `integrity-final.json`。
- 102 项覆盖矩阵已生成 `coverage-matrix-final.json`：90 PASS、4 PARTIAL、8 NOT COVERED。PARTIAL 为 M04（401/429/不可达已测，超时未完成）、O06/X11（更新撤销与冲突保护已测，全部变更类型的 UI 撤销驱动未完成）、X10（命令保护及搜索成功/错误/截断已测，在途搜索取消未测）。D05、D09 和 G01–G06 明确未覆盖，不能把强杀或本机测试替代优雅停机、精确转交崩溃窗口或云部署验收。
- 撤销矩阵首轮及复跑的新增失败均是驱动定位/会话选择问题；冲突场景通过并明确显示“文件已被其他操作修改，请重新加载或确认保留本地修改”，人工版本未被覆盖。既有 `acceptance-fixes` 的真实修改撤销、刷新后状态和二次撤销禁用仍是 O06/X11 的有效证据。保留 `undo-matrix.log`、`undo-matrix-followup2.log`，不把驱动失败算作产品缺陷。

### 交付判断

当前构建满足本地文稿工作台的核心可靠性验收：结构化工具能真实读写、事件和文件效果一致；用户隔离、幂等、取消、队列、SSE 补读、后台任务追踪、恢复保护、编辑冲突和数据完整性均有证据。平台没有观察到真实上游 HTTP 故障造成的错误；隔离 401/429/502/不可达结果都能有界失败并保持可继续使用。

仍需在下一轮单独修复/验收：统一后台提示词与工具 schema 的编辑协议（AC-01，P2）；将 provider/model 归属补入后台失败诊断（AC-02，P3）；把禁用命令的标签从“已运行命令”改为明确的拒绝/失败文案（AC-03，P3）。这三项尚未修改源码，因此本报告记录为已确认缺陷而不是“已修复”。

本轮未推送、未部署、未发送外部消息、未删除 8080 用户数据；测试脚本和证据均在 `_local_artifacts/agent-full-function/20261023-1215/`。结论只针对 Windows 单进程本机构建和列出的场景，不承诺零缺陷；云端/容器/优雅停机/完整后台 checkpoint-resume 与多副本 SQLite 运维能力仍需独立工程验收。

## 逐条回复与异常归因

真实流程共审阅 **84/84 条去重后的可观察正文**；保留被终态更新的段落及后台轮次，分支/重放副本建立关联，没有把 token 数当回复数，也不声称审阅了未公开推理。逐条记录在 `reply-review.json` / `reply-review.md`。

- 75 条正常；4 条属于模型内容偏差（主要是 steer 后对材料来源的理解偏差）；2 条模型错误在有限步骤内修正；没有发现无界绕圈。
- 1 条真实编辑已写入并回读，但模型未返回合法完成协议，平台明确 failed，保留文件，没有假报整轮成功。不能因写作效果或协议未遵循就称平台丢文件。
- 1 条最终核对受测试驱动错误描述污染：第一次 stale 注入前 SQLite_BUSY，实际上没有完成外部写入，后续驱动却告诉模型已发生。保留首次现场并单列；第二次真正读后注入已通过，人工新版本完整保留。
- 1 条是预期旧版本冲突后的 partial，属于保护生效。

本轮真实服务没有观察到需要归咎于上游 HTTP 的故障；401/429/502/超时使用隔离供应商注入，不能混称真实上游现场。

测试驱动失败保留，不覆盖原始日志：队列最初错误假设同用户并行；UI 旧撤销文案、同名任务/会话、富文本 Markdown 转义、表头计数、缩放截图裁剪、格式修复弹层遮挡等已分别纠正。以 `*-followup*`、`*-verified*` 的定向复验为准，不通过重复跑全套掩盖失败。

## 尚待完成与边界

102 项用例的最终矩阵、90 分钟测试资源趋势和最终原数据核对将在收尾填入。本机不具备经授权的云端/Linux/容器/HTTPS 反向代理环境；G01–G06 明确未覆盖。Windows 真正优雅停机没有验证，强杀测试不能替代。转交已测 HTTP 全链路数据库故障及显式恢复，没有在任务创建和回执之间的精确间隙强杀进程，D09 不能整项通过。搜索成功/提取/截断/无配置与权限有证据，但可控搜索服务的在途取消尚未覆盖。结论只适用于记录的 Windows 单进程构建和本轮场景。

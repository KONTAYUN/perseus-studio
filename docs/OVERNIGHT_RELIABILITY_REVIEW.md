# 夜间逐项审计与修复

> 后续交付（2026-10-23）见 [FINAL_ACCEPTANCE_FIXES.md](FINAL_ACCEPTANCE_FIXES.md)：当前 8080 使用 `_local_artifacts/perseus-final-fixes.exe`，数据仍在 `manual-runtime`。本文下述 2026-10-20 版本及夜间安排为历史记录，最新测试证据与备份以交付报告为准。

> 当前交付版本为 07:56 的 `_local_artifacts/perseus-handoff-20261020.exe`，8080 沿用原手动数据库与用户文件。04:09 和 07:51 是此前快照；最后的收尾记录包含本次新增生命周期保护、保守后台恢复与恢复演练。

开始时间：2026-10-20 03:05（Asia/Shanghai）。用户授权继续自主审计、测试、实现适用的成熟 Agent 机制。不得输出密钥、丢弃现有改动、操作真实用户测试数据，或将未验证能力写成已完成。

已设置本任务 heartbeat，每 30 分钟检查继续，至北京时间 08:00 截止。当前持续工作优先；heartbeat 用于后续接续，不代表已经持续运行或验证完成。

## 前序基线

见 PROJECT_OPTIMIZATION.md：129 前端单测、15 浏览器测试、后端全量、vet、Windows 进程取消、Linux 交叉编译通过。手动实例 8080 的二进制为 `_local_artifacts/perseus-optimized-release.exe`，数据在 `manual-runtime`；原配置保留。不能直接用该实例运行会创建默认模型的自动化回归。

## 本轮研究与工作顺序

1. 读取官方资料对照权限、隔离、检查点、撤销、工具契约、调度与状态恢复。
2. 修复可复现的文件竞争、取消或事件恢复缺陷，然后补回归。
3. 实现文件变更审查/按轮安全撤销的最小闭环，并逐步收敛聊天运行状态。
4. 检查 API/存储/命令资源限制、发布门禁和备份恢复。
5. 全量回归、独立实例故障注入、真实模型小规模验证；确认无活动任务后更新 8080。

## 已完成修复（03:38 接续记录）

- 文件写入、移动、排序和回收站共用互斥锁；补丁移动/删除在提交时重新比较哈希，并检查取消。禁止根路径别名、NTFS ADS 和链接回收站迁移。原子写入保留权限，元数据失败补偿移动；覆盖恢复先保存当前内容到回收站。
- 修复 `trash_items.id` 用相对路径作全局主键造成的跨用户同名记录覆盖，改为独立 ID 和按用户路径更新。历史已经丢失的原路径无法凭空还原。
- 消息、运行、初始事件和请求身份在同一 SQLite 事务准入；同会话活动运行互斥。正常发送支持幂等键与响应丢失后的身份查询。
- 重新生成进入统一队列/账本/取消流程，准入成功后才截断旧答案；恢复拒绝过期运行。前端补停止时尚未拿到运行身份、异步跨会话回写、网络请求超时处理。
- 新增基于服务端工具账本的逐文件变更查看/安全撤销。哈希冲突拒绝撤销；副作用前记录 pending，完成记录 completed，未知结果拒绝盲重试。撤销后磁盘、文件树和编辑器同步。
- API 请求体、聊天附件、Skill 压缩包、文件读取和写入增加大小限制，哈希采用流式读取。

逐文件撤销仅覆盖工具记录和可用快照，不是整轮工作区检查点，也不支持 shell 回滚。预览每文件最多 512 KiB；完整磁盘/数据库崩溃事务和 OS 沙箱尚未实现。

## 收尾新增检查

- 修复恢复状态已变 queued、恢复事件写入失败后任务无法调度的问题：两项写入同事务。恢复标记携带本次请求身份，HTTP 响应丢失后不再重放以前的失败事件而误停新连接。
- 排序跨目录移动失败和回收站恢复元数据失败会补偿实际文件位置；覆盖恢复的两份内容均有故障注入回归。目录后代匹配改用字面、区分大小写的前缀，修复 `%`、`_` 或大小写目录名误改别处排序记录。
- 撤销前重新计算基线快照哈希；快照内容损坏时拒绝恢复。新增稀疏超大文件/超限写入回归，确认拒绝后不改原文件、不创建新目录。
- 子进程改为继承运行环境白名单并使用用户独立 command home，不继承服务密钥、代理认证、loader/Git 注入变量；实际 PowerShell 子进程回归通过。
- 启动前取得数据库和用户目录的 OS 文件锁。重复启动在迁移和 stale-run recovery 之前失败；同进程竞争、跨进程竞争、进程被终止后重新取得锁均有 Windows 回归。
- 修复消息操作按钮点击受阻、键盘/触屏操作可见性、单栏窄屏强制撑宽和多栏窄屏内容无法滚动到达。标签拖拽使用本面板拖拽身份，避免在浏览器 protected dragover 阶段读取不到 payload。
- 旧运行结束后的标题轮询曾反复清空后续任务的错误/恢复入口。列表刷新现保留运行状态；标题刷新拥有可取消计时器与请求，生成成功后停止，替换/卸载时清理。前端完整单测从约 46 秒降至约 0.24 秒，是清理遗留异步工作的结果。
- 可选标题推理从未追踪 goroutine 改为独立有界队列（1 worker / 16 admissions），按会话去重，不占前台执行槽，停机取消并等待。此队列不承担工作区修改。

## 验证与实例状态（04:09 交付快照）

- 前端单测 **138 通过**（最终约 0.21 秒）；最终代码 Go 全量与 vet 通过，Linux amd64 交叉编译通过。构建、UI 和结构门禁通过（119 前端文件、273 Go 文件）。最终日志见 `_local_artifacts/overnight-handoff-{unit,go,vet,e2e}.log`。
- 完整 Chrome E2E **16 通过、1 个真实模型 opt-in 跳过**，最终前端版本再次通过。包含 OpenAI/Anthropic 编辑→预览→人工冲突保护→撤销→编辑器刷新、丢失准入响应且不重复消息、重新生成进入持久 run、准入期间停止、自动保存、真实标签拖拽、菜单及快速布局/减少动画。浅色、深色窄屏截图均已查看。
- 独立真实模型首次回归 **2/2 通过**：`grok-4.3`（约 31.5 秒）与 `grok-4.20-0309-non-reasoning`（约 16.2 秒）。随后最终版本复测遇到中转连续 502/503，两个运行在有限重试后明确失败；小请求确认服务恢复 200 后，最终版本 `grok-4.3` 再次完整通过（约 34.5 秒）。三种文件和编辑器刷新均核验。不能将供应商偶发故障写成全程绿灯。
- 真实测试日志分别为 `overnight-live-e2e.log`、`overnight-handoff-live-e2e.log`（503 故障）、`overnight-live-recovered-e2e.log`。当前成功脱敏证据为 `overnight-live-evidence.json`，故障脱敏证据保留为 `overnight-live-503-evidence.json`。凭据仅由内存代理读取，未写入自动化供应商或报告。
- **8080 已于 04:05 升级至最终版本** `_local_artifacts/perseus-stable-20261020.exe`，exec session `17822`，PID `109364`。原数据库和用户文件保留在 `manual-runtime`。升级前确认无 queued/running/pending 任务，离线完整备份在 `_local_artifacts/manual-backups/20261020-0405/`；原二进制也保留。
- 升级前后数据库 quick_check 正常；1 个账号、1 个原供应商配置和 1 个用户文件的完整摘要一致（忽略运行锁文件）。登录、原 `grok-4.3` 配置、布局切换通过，浏览器无异常，`/api/health`、`/api/ready` 正常。手动账号沿用 `perseus`，不在报告记录密码或 key。
- 独立 18080 实例完成最终全量模拟 E2E 和真实模型验证后停止。后续需要故障复现时用 `optimization-runtime` 单独启动，不在手动数据库创建 mock 供应商。
- [成熟 Agent 固定源码对照与后续验收](MATURE_AGENT_COMPARISON.md) 已完成。README 记录请求恢复、资源限制、逐文件撤销和真实隔离边界。
- 尚未验证：Docker engine 未运行，不能声称完成真实容器测试；本机缺少可用 C 编译器，race 检查交由已配置 CI，本地未执行。Linux 仅交叉编译，未运行 Unix 权限/进程测试。
- 当前已定位的本轮缺陷均有修复与对应验证。后续未实现的边界包括 OS worker 隔离、整轮快照/多文件崩溃事务、分布式调度、磁盘配额和长期压力/备份恢复演练，详见对照报告，不能视为已解决。

## 后续 heartbeat 接续

已有 `perseus` heartbeat 每 30 分钟接续，至 08:00；不要重复创建。保留 8080。无新证据时保持安静，不制造改动或重复全量测试。发现新异常才做最小复现、修复和适当回归。真实供应商异常先用小请求确认可用性，记录外部 5xx；不要用无限重试掩盖失败。新版本使用新二进制文件名，不覆盖正在运行的 exe。任何再次升级均先核对活动任务、备份并保留当前可用版本。

## 研究来源

- OpenAI 官方：`https://developers.openai.com/codex/agent-approvals-security`、`https://developers.openai.com/codex/noninteractive/`，已实际读取。权限策略与 OS sandbox 分层、结构化运行事件、自动化认证边界值得对照。
- Claude Code：`https://code.claude.com/docs/en/checkpointing`，已读正文；用户输入前检查点、代码与会话分开恢复，shell/外部会话/链接修改不保证回滚。核心产品并非全部开源，不能按可审计开源运行内核描述。
- Roo Code 固定 HEAD：`b867ec9145750d0ae1ff7f02d35406e9bf2a0b16`。
- OpenClaw 固定 HEAD：`59c74d52601820df7f751b89391acc34e187cd87`。
- Codex 固定 HEAD：`49df716ba92cf49a44140241b366d914c27cfc80`。已读 sandbox adapter、rollout recorder 与 writer lock：有界写队列、flush/shutdown 确认、待写后缀保留和跨进程所有权；未声称审计完整产品。
- Roo 已读 shadow Git checkpoint 实现：独立仓库、环境净化、提交快照和恢复；不能把其 hard reset 直接套到用户工作区。OpenClaw 已读队列 lane/取消/排空和 session writer 生命周期所有权校验；旧 session write lock 文件为 deprecated no-op，不作为当前机制证据。
- GitHub API 当前公共出口限流，已通过只读 Git shallow/filter checkout 和原始文档继续研究。参考仓库位于忽略目录 `_local_artifacts/overnight-research/`，未运行其中代码。

所有结论只针对已读机制与已跑测试；不承诺穷尽全部缺陷。

## 07:50 最终收尾审计

- 核对工作树、04:09 的验证日志和手动实例：`/api/ready` 正常，运行/排队均为 0；原用户、供应商配置与工作区文件摘要保持一致。没有把未实际执行的 heartbeat 间隔描述为持续测试。
- 新增独立数据库故障回归，确认会话删除会移除仍被 queued/running 聊天和 pending/running 后台任务使用的消息；并发准入测试实际产生了无会话的活动 run。修复前失败证据：`_local_artifacts/overnight-delete-before.log`。
- 删除现与活动运行/关联后台任务检查同事务；后台 AI 任务创建在事务内检查会话归属和存在性；恢复运行要求会话仍存在。API 对仍注册的执行器也拒绝删除，包括已标记 canceled 但尚未清理完的运行。活动删除返回 HTTP 409。原终态运行账本保留，删除不是撤销工作区副作用。
- 新增 6 项回归，覆盖 queued/running、pending/running、取消清理、跨用户不删除、20 次前台准入竞争、12 次后台准入竞争、删除存储失败回滚和孤立恢复拒绝。调整两处原恢复测试的虚构会话夹具，使其使用真实会话。专项通过：`overnight-delete-after.log`。
- 新代码 Go 全量通过：`overnight-delivery-go.log`；最后补充取消清理保护后 API/store 两包全量再次通过：`overnight-delivery-final-api-store.log`。vet、构建、UI/结构门禁通过（119 个前端文件、277 个 Go 文件）。前端生产代码未在本次收尾更改，沿用 138 项单测与已构建 dist。
- 使用 04:05 离线备份在 `_local_artifacts/restore-drill-20261020/` 和独立端口 18081 演练恢复，SQLite quick_check 与 foreign_key_check、实际登录、工作区文件读取哈希、用户和原供应商摘要全部通过；未调用供应商，未修改生产数据。演练服务已停止。证据：`overnight-restore.log`、`overnight-restore-evidence.json`。此演练覆盖当前小规模备份，不等于 WAL/大库/磁盘故障恢复均已验证。
- 最终后端在独立 18080 数据库重跑 Chrome：**16 通过、1 个真实模型 opt-in 跳过**（28.7 秒），`overnight-delivery-e2e.log`。本次生命周期修改未新增真实模型调用；真实 Grok 成功及中转 502/503 的证据仍以前文为准。Linux amd64 CGO0 再次交叉编译通过，仍未执行 Linux/race/容器运行测试。
- **07:51 手动实例再次升级**至 `_local_artifacts/perseus-stable-20261020-final.exe`（PID `142340`，exec session `56498`）。升级前再次核对队列与持久任务均空、文件/用户/配置摘要一致，停止旧进程后备份匹配数据库与用户目录至 `_local_artifacts/manual-backups/20261020-0751/`。原 04:05 二进制和备份均保留。升级后登录、原模型配置、布局、0 browser errors、数据库检查和所有摘要比对再次通过；日志 `overnight-delivery-manual-ui.log`。独立 18080 测试服务已停止，测试数据保留。

## 07:57 后台重启恢复收尾

- 复核发现后台 AI 运行只带 TaskID、没有前台 RunID 的持久检查点契约，而原 `RequeueRunningTasks` 在启动时把所有 running 任务改回 pending。这可能重复中断前的命令/文件副作用；旧逻辑还会在日志插入失败时留下已修改状态。新增两项独立数据库故障测试均先复现失败，证据 `overnight-task-restart-before.log`。
- 启动恢复改为同事务记录日志和状态：中断的 AI/未知任务明确 failed / recovery_required，提示核验产物后新建任务，不自动重放；已有取消请求保持 canceled；只有无模型副作用的 mock 任务允许自动重排。尚未执行的普通 pending 任务保留。第二次恢复不会重复记录。后台任务完整 checkpoint/resume 尚未实现。
- 最后代码 Go 全量再次通过：`overnight-delivery-go-final.log`；vet 通过：`overnight-delivery-vet-final.log`。最终普通生产代码结构门禁通过，扫描 119 个前端文件、279 个 Go 文件；Windows 构建及 Linux amd64 CGO0 交叉编译通过。最后一项只涉及启动时存储恢复，浏览器全套沿用此前 16 通过/1 跳过，未声称为最后这项重复运行浏览器全套。
- **最终手动版本** `_local_artifacts/perseus-handoff-20261020.exe`，07:56 启动，PID `139540`、exec session `2810`。更换前确认持久运行/任务与队列均空、原始摘要匹配，离线完整备份 `_local_artifacts/manual-backups/20261020-0756/`。更换后数据库检查、原用户/模型/文件摘要、实际登录、布局、0 page errors 和 ready 全部通过，证据 `overnight-handoff-manual-ui.log`。8080 保留运行，18080/18081 已停止，数据未删除。
- 最终保留风险：宿主命令没有 OS 文件/网络沙箱；没有整轮快照、多文件跨文件系统/数据库的崩溃事务、后台 AI checkpoint/resume、分布式接管与磁盘配额；长期负载、大型/WAL 备份及故障恢复仍需专项验证。Docker engine、Linux 原生运行和本地 race 验证仍不可用。当前已复现问题修复不等于零缺陷保证。

## 08:00 夜间工作结束

北京时间 2026-10-20 08:00 完成交付核对并停止夜间自动接续，`perseus` heartbeat 已暂停。8080 手动服务保留运行，最终 ready 正常且运行/排队均为 0。未推送、未远端部署、未发送外部消息、未删除用户数据；全部修改保留在当前工作树。后续应先推进受限执行 worker 与故障阶段恢复验收，再考虑更多功能。


## 后续授权整改（2026-10-20 至 2026-10-21，与已结束的夜间任务分开）

用户在整体设计审计后明确批准实施；本次是该请求的收尾，不是重新启用 08:00 已停止的 heartbeat。逐项 A01–A16 的实现、证据和未关闭范围见 [DESIGN_AUDIT_REMEDIATION.md](DESIGN_AUDIT_REMEDIATION.md)。

- 移除正文补丁执行及本地完成关键词硬判，使用结构化工具/outcome、错误原因链、明确能力/范围；补齐终态消息收敛、严格匹配、递归参数验证、持久化失败取消、命令结果与统一运行预算。
- 多文件补丁新增持久原文/预期哈希恢复计划；子进程在四个阶段直接退出的恢复测试通过，人工后续修改冲突时保留内容并停止。仍不等于 SQLite/文件系统 ACID 或断电恢复保证。
- 收尾又修复真实 rename/move 参数字段的目标范围遗漏、空格文件名不能用显示归一化混过授权、目录递归和动态整理不能绕过精确文件范围；补充零副作用反例与允许操作正例。失败命令的结果跨压缩/检查点保存，无关读不能清除。
- 最终 Go 全包测试/vet、152 项前端单测、UI/结构门禁和 Windows/Vite 构建通过（135 前端/300 Go 文件（扫描含测试））；完整 Chrome 26 通过/1 opt-in 跳过，真实 Grok 编辑一项通过；最后边界小修后最终构建专项再跑 3/3 通过。日志执行时点在整改报告中明确，不把早期全套测试宣称为最后代码重复执行。
- 2026-10-21 00:06 更新空闲的 8080，当前二进制 perseus-design-audit-20261020-final.exe / PID 26424。离线匹配备份 manual-backups/20261021-design-audit-final；原 22 表511行与3文件摘要、SQLite完整性、实际登录/模型/布局和0页面异常全部通过。18080 已停止，8080 保留运行，没有推送或删除用户数据。
- 宿主命令默认关闭，trusted_host 仅显式启用且管理员可调用；OS 沙箱仍未交付。完整前端 reducer/ToolSpec、Linux原生/race/容器、断电/长负载验收仍待完成。移除了以数字高分代替可靠性判断的健康自评，改为能力/故障矩阵；构建大 chunk 警告仍如实保留。


### 00:11 末轮恢复日志完整性收尾

- 故障注入确认：合法 JSON 内的恢复原文损坏会被恢复写回，缺少与 BeforeHash 的核验。现先验证整份计划的原文哈希/路径/操作/预期哈希格式，再进行任何逆序写入；损坏时保留 pending 证据并拒绝全部写入。独立库先红后绿日志 design-fix-journal-integrity-{before,after}.log。
- 最后代码再次通过 Go 全包（design-fix-delivery-go.log）、vet（design-fix-delivery-vet.log）、结构门禁（135 前端/300 Go 文件，扫描含测试）及 Windows 构建。最终二进制在独立实例再跑两协议编辑/外部冲突保护 3/3，design-fix-delivery-edit-e2e.log；前端未变，152 项单测与构建沿用本轮最终记录。
- **当前 8080** 为 perseus-design-audit-20261021-verified.exe，PID 62968，00:11 启动。升级前再次核对任务空闲并完整离线备份 manual-backups/20261021-journal-integrity-final/；23 表512行（含新迁移表和前次登录 session）及3文件摘要、数据库检查、实际登录/配置/布局与0页面异常均通过。证据 design-fix-delivery-manual-{integrity,ui}.log。独立18080再次停止，8080继续运行。

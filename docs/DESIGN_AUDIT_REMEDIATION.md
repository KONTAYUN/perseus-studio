# 设计审计整改与验收

审计日期：2026-10-20；本轮整改收尾：北京时间 2026-10-21。对象是当前本地工作树，包含此前未提交的修改。用户已授权实施 A01–A16 整改。本报告取代原审计中“尚未实施”的状态判断，原反例保留作历史证据；不代表穷尽全部缺陷。

## 本轮实际改变

取消用正文关键字决定动作与终态的主要路径，改由结构化工具、任务 outcome、保留原因链的错误和验证事实驱动。没有更换框架或大搬目录。前后端仍使用现有模块边界，两个协议共用运行器初始化。

| 编号 | 当前交付 | 回归 / 验收证据 | 剩余范围 |
|---|---|---|---|
| A01 动作分派 | 已修复：正文和围栏补丁只展示；结构化调用不再被正文示例替换；两协议保留结构化 patch 工具 | 独立库讲解示例零写入、混合正文/真实工具、OpenAI SSE / Anthropic JSON 浏览器编辑 | 旧补丁信封解析 API 留作格式解析与历史测试，运行分派不调用文本转动作；不恢复隐式执行兼容 |
| A02 终态 | 已修复关键词硬判；新增 success/partial/blocked、实际变更核验，success 与 failed_items 冲突时拒绝；API 消息 metadata / run_completed 保存 outcome | 说明工具名/“尚未”不被误拦；部分完成可结束；未验证路径与矛盾 outcome 拒绝 | outcome 是结构化声明加工具事实约束，不证明模型所有自然语言描述正确；run succeeded 可对应 task partial |
| A03 错误身份 | 已修复本地主要链路：Code/Cause/Unwrap 与 errors.Is/As；不再把原 error 变为文字后重新猜；事件携带 failure_class | 错误包装、翻译文案与显式码冲突反例；文件缺失与路径策略错误 | 不是所有旧错误都新增专用码；未知错误保守归 generic。外部 provider 文字兜底仍限于适配边界 |
| A04 provider 能力 | 已去掉显示名、模型别名、URL 对上下文/输出预算的推断；明确 context override 与持久 system_message_mode，设置页可选 single/multiple | 更名不改变 profile；能力配置保存、更新和非法枚举拒绝；真实 Grok 验证 | 不是完整自动能力协商。旧 Claude Code 代理的 URL 路由/认证适配仍在；没有把它宣称为全部显式化 |
| A05 消息恢复 | 已修复：终态快照按 run 身份权威收敛，较短最终回复覆盖旧草稿，已撤回草稿清除；保留队列用户输入和更新的活动 run | 新增 3 项纯事件/快照合并反例；已有浏览器丢响应与恢复回归 | 完整断网时序组合与全量 reducer 迁移仍待扩展；不是所有事件序列的形式证明 |
| A06 匹配语义 | 已修复：旧文本编辑和补丁只做精确内容匹配，保留歧义拒绝、CAS、候选建议；合法“其余不变”正文不再被关键词误拦 | 缩进、Unicode 引号、NBSP 不匹配时两文件均不写；合法字面正文可编辑 | 换行风格兼容保留；模糊候选仅帮助重读，不决定落盘位置；工具不替代编译器 |
| A07 命令权限 | 已缓解：默认关闭；仅 trusted_host 显式启用且管理员可用；执行器自身也拒绝未启用；移除假隔离禁词表 | 默认禁用、普通用户绕过拒绝；命令测试显式 opt-in | **OS 沙箱未实现，条件 P0 未完全关闭**。启用即服务账号宿主权限，环境净化不是磁盘/网络隔离 |
| A08 持久化失败 | 已修复：首个 API 持久化失败先取消执行所有者，再记录诊断；新工具检查 ctx；保留原始故障而非误报用户取消 | 独立 SQLite trigger 只阻断 tool_call_completed；首个已发生副作用可核验，第二文件和第二请求不发生 | 不能撤回故障前已经发生的外部副作用；诊断本身也可能因存储故障不可写 |
| A09 崩溃恢复 | 已实现持久 patch 原文/预期哈希计划（最大 16 MiB），成功后确认；按反序幂等恢复；用户后续修改冲突时停止；保留回收站副本；整份恢复计划先校验原文哈希再写 | 子进程在 write/create/move/delete 后直接退出，重开库恢复；二次恢复；后续人工内容保留；合法 JSON 内原文损坏时全计划零写入 | **不是文件系统/SQLite ACID、断电保证或整轮快照**。恢复可能留空目录/历史版本；人工编辑与其他程序并不被全局锁住 |
| A10 参数契约 | 已实现已发布子集的递归 type/required/enum/items/properties、额外属性和数值/长度边界；深度上限 32 | 真实 schema 的数组坏元素、嵌套 enum/必填/布尔步骤拒绝 | 不是完整 JSON Schema；统一 ToolSpec 生成仍未完成 |
| A11 命令结果 | 已区分调用成功与 execution_outcome；允许 expected_exit_codes；failed/timed_out/canceled/observation_incomplete 不当作验证通过；失败命令按 cwd+原命令持续记录 | 实际 exit 23；无关读不能清除；大结果/历史压缩保留结果；检查点与精确成功重跑 | 不能判断测试内容本身是否充分；模型若改变命令，不视作同一失败已解决，可诚实 partial/blocked |
| A12 引用/范围 | 引用恢复为上下文；独立 AllowedWorkspacePaths，nil=普通策略，空集=禁止；公共工具入口复核；恢复取交集不扩大；rename/move 检查真实目标字段 | 新增独立库源/目标越界零写入、文件名空格/绝对路径反例、允许文件移动/重命名、目录递归拒绝；检查点空范围保留 | 显式范围暂为运行器接口，未做完整授权 UI；只限制新工具目标，不抹除已有聊天/附件上下文；必要父目录和版本/回收站元数据不算独立文件授权。命令、动态整理等不能执行精确范围的工具直接拒绝 |
| A13 扫描边界 | 命令超时先于扫描；扫描检查取消，5 秒/1 万项/256 MiB 哈希上限；后置扫描独立 3 秒；读取失败与观察不完整显式返回 | 已取消、缺失文件读错误、257 MiB 稀疏文件预算拒绝 | 没有大工作区吞吐基准或任意宿主副作用追踪；这是观察边界，不是进程资源隔离 |
| A14 结构/进展 | 部分完成：共用初始化；语义结果摘要识别只读进展、过滤时间戳；既往修改不再关闭重复读保护；循环状态持久化 | 同内容反复读会停、持续真实内容变化不会误停、时间戳不算进展 | **前端 reducer / 强类型全事件迁移未完成**；保留 facade 接口逐步迁移，不以机械拆文件冒充解耦 |
| A15 重试预算 | Retry-After 不再截短后提前请求；跨轮、重试、回退、自动恢复共用请求/时间预算；预算失败不自动恢复 | 60 秒等待语义、超出 deadline 不提前重试、实际 HTTP 跨轮限额、溢出等待值 | 不含金额与实际计费 token 预算；主动 resume 是新 invocation；optional title 有自己的有界生命周期 |
| A16 质量证据 | 将反例转长期回归，以行为/故障矩阵代替数字高分；保留旧基线和中间失败日志 | 见下表及健康证据文档 | Linux 原生、race、容器、断电、大库/长压测没有本轮执行证据，不能视为通过 |

## 最终验证

所有故障测试使用独立临时数据库/目录。浏览器 mock 与真实模型验证仅针对 18080 的 design-audit-validation 数据，不在 8080 创建测试供应商或文件。

| 检查 | 实际结果 / 本地日志 |
|---|---|
| 最终 Go `test ./... -count=1 -timeout 180s` | 全包通过；`_local_artifacts/design-fix-delivery-go.log` |
| 最终 `go vet ./...` | 通过；`design-fix-delivery-vet.log` |
| 前端单测 | 152/152；`design-fix-final-unit.log` |
| UI / 结构 | 通过；135 前端 / 300 Go 文件（Go 扫描含测试），普通生产文件 <450 行 |
| 最终 Vite build / Windows 构建 | 通过；`design-fix-final-build.log`；仍有约 554/648 kB chunk 提示，未隐藏警告 |
| 本轮完整 Chrome E2E | 26 通过、1 个 opt-in 跳过；`design-fix-e2e.log`，在最后 scope/命令状态小修前执行 |
| 本轮真实模型 | grok-4.3 一项通过（现有/空/新文件落盘与刷新）；`design-fix-live-e2e.log`，在最后 scope/命令状态小修前执行；不外推所有模型 |
| 最终构建编辑专项 | 3/3 通过（OpenAI、Anthropic 编辑与外部内容自动保存冲突保护）；`design-fix-delivery-edit-e2e.log`（最后恢复日志完整性修复后重跑） |
| 本轮未执行 | Linux 原生 / race / Docker 运行、长压测、断电及任意并发进程干扰；历史交叉编译不等于本轮运行验证 |

永久新增测试主要在 `backend/internal/ai/design_{audit,execution,boundary}_regression_test.go`、`runtime_policy_checkpoint_test.go`、`backend/internal/workspace/patch_journal_test.go`、`patch_journal_integrity_test.go`、`backend/internal/api/run_persistence_fault_test.go` 和 `tests/unit/chatMessageMerge.test.js`。原来要求错误行为的夹具已改为真实结构化动作，没有用通用 mock 自动把正文补丁执行来掩盖协议变化。

## 手动实例与交付边界

北京时间 **2026-10-21 00:11**，8080 已更新为 `_local_artifacts/perseus-design-audit-20261021-verified.exe`，PID **62968**；前端使用 `_local_artifacts/frontend-design-audit-20261020-final`。继续使用原 `manual-runtime/app.db` 和用户目录，宿主命令保持默认关闭，文件工具可用。

更换前确认数据库与调度器均无运行/排队任务；先记录摘要，停机后再次记录，二者一致。离线完整数据库、用户目录、原二进制和前端备份位于 `_local_artifacts/manual-backups/20261021-design-audit-final/`。新进程启动后、浏览器登录新增 session 之前，原 **22 张表 / 511 行**按原列投影的摘要及 **3 个用户文件**的 SHA-256 全部一致，SQLite quick_check / foreign_key_check 通过，迁移新增 system_message_mode 的兼容默认值已核验。第一次切换证据 `design-fix-manual-integrity.log`；清单仅存哈希，不存密钥或聊天正文。

实际浏览器登录、原模型配置、布局切换、动画回收均通过，**0 page errors**；`/api/ready` 正常、running=queued=0。最终证据 `design-fix-delivery-manual-ui.log`。独立 18080 实例已停止，8080 保留供用户使用。全程没有推送、远端部署、发送外部消息或删除用户数据。新建数据库列属于明确迁移，不能因为多一列就误报数据丢失；登录新增认证 session 属于本次验收的正常记录。

最后复查补充了一项真实故障：有效 JSON 中的恢复原文损坏会被静默写回。修复前复现日志为 design-fix-journal-integrity-before.log；修复后整份计划先验证路径、操作、预期哈希格式和原文哈希，任一无效即保留 pending 日志、拒绝全部恢复写入。专项通过 design-fix-journal-integrity-after.log，随后最终 Go 全包/vet 和两协议浏览器编辑再跑通过。

00:11 更换前再次确认空闲并离线完整备份到 manual-backups/20261021-journal-integrity-final/，包括前一二进制和相同前端。此次基线 **23 表 / 512 行 / 3 文件**（包含迁移的新表和前次 UI 验收新增 session）；切换后所有原列摘要与文件哈希完全一致，数据库检查和登录再次通过，见 design-fix-delivery-manual-integrity.log。两次备份均保留。

下一阶段先做受限 OS worker 的文件/网络/资源验收，再做聊天 reducer/统一 ToolSpec，随后补 Linux/race/容器与负载恢复矩阵。继续以可复现故障驱动小步改动，不承诺未经验证的零缺陷。

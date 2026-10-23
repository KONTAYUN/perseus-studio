# 与成熟 Agent 的实现对照

审阅日期：2026-10-20。对照固定版本的具体机制，不以产品名、功能数量或主观分数代替证据。Claude Code 的核心产品不是全部开源，以下对它仅引用官方行为说明。

## 结论

Perseus 的主要问题不是 React、Go 或 SQLite 选错了，也不是目录必须全部重写。已有组件、hooks、API、store、provider、workspace 边界可以继续使用；早期感主要来自运行状态、取消、落盘和编辑器同步之间缺乏统一所有权，以及失败后无法确认副作用。此次优化沿现有边界补闭环，避免同时引入第二个执行器或一套新状态系统。

当前生产文件均在 450 行门禁以内。行数只是一项维护约束；仍需关注 provider 消息转换、工具 manifest、聊天流状态和运行执行这些接近预算、规则集中的模块。它们应随具体功能逐步拆分，不能仅为好看改文件名。

## 机制对照

| 成熟项目与已读证据 | 成熟之处 | Perseus 本轮落实 | 尚未达到的边界 |
|---|---|---|---|
| [Codex writer ownership](https://github.com/openai/codex/blob/49df716ba92cf49a44140241b366d914c27cfc80/codex-rs/rollout/src/writer_lock.rs) / [rollout recorder](https://github.com/openai/codex/blob/49df716ba92cf49a44140241b366d914c27cfc80/codex-rs/rollout/src/recorder.rs) | 写入者持有跨进程锁直到后台 I/O 完成；有界写队列、flush/shutdown 确认和失败状态；写入失败保留未写后缀 | 服务在迁移、恢复前取得数据库和用户根目录的 OS 锁；单用户任务串行；运行/输入/初始事件事务准入；恢复状态与标记同事务；SSE 以 SQLite 序号补读 | 当前仅支持单服务所有者；没有分布式工作者租约和代次 fencing。SQLite 事务不等于文件系统事务 |
| [OpenClaw command queue](https://github.com/openclaw/openclaw/blob/59c74d52601820df7f751b89391acc34e187cd87/src/process/command-queue.ts) / [writer revocation](https://github.com/openclaw/openclaw/blob/59c74d52601820df7f751b89391acc34e187cd87/src/agents/embedded-agent-runner/run/session-writer-revocation.ts) | lane、容量、排空和取消构成明确生命周期；会话写入按 session ID、revision 和当前 run 校验，旧清理不能覆盖新所有者 | 前台聊天、重新生成、后台任务和撤销共用调度；准入有界，取消可覆盖尚未收到 HTTP 身份的阶段；旧运行恢复和迟到终态受保护 | 尚无持久调度租约、优先级/截止时间策略、跨机器接管。旧 deprecated session-lock no-op 文件不作为有效实现证据 |
| [Roo shadow checkpoints](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/services/checkpoints/ShadowCheckpointService.ts) / [checkpoint docs](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/apps/docs/docs/features/checkpoints.mdx) | 独立 shadow Git 管理快照，净化 Git 环境，提供可查看/恢复的历史；恢复范围和忽略规则明确 | 工具变更记录内容哈希、快照、移动源和回收站位置；可查看并撤销单个文件变更；人工修改冲突时拒绝覆盖；撤销未知结果不盲重试 | 没有完整工作区/整轮检查点。未照搬 hard reset/clean，因为它可能破坏人工文件和未保存内容；shell 与大文件不能通过此入口回滚 |
| [Claude Code checkpointing](https://code.claude.com/docs/en/checkpointing) | 输入与文件恢复形成用户可理解的闭环，区分会话恢复和代码恢复；说明 shell、其他会话与链接修改的限制 | 重新生成明确提示不撤销已执行修改；原答案在准入失败前保留；逐文件审查和撤销不混同于重新生成 | 尚无代码/会话分开 rewind、整轮快照管理、快照保留策略；不能将其全部核心实现视为开源可审计 |
| [Codex sandbox policy adapter](https://github.com/openai/codex/blob/49df716ba92cf49a44140241b366d914c27cfc80/codex-rs/core/src/sandboxing/mod.rs) / [OpenClaw isolation distinctions](https://github.com/openclaw/openclaw/blob/59c74d52601820df7f751b89391acc34e187cd87/docs/gateway/sandbox-vs-tool-policy-vs-elevated.md) | 工具允许与否、权限提升、OS 文件/网络隔离分层；不能用命令黑名单代替隔离 | 子进程只继承运行环境白名单，使用用户独立 command home，不继承服务密钥/注入钩子；命令输出、超时和后代取消有限制 | 命令仍以服务身份在宿主执行，能访问该身份可见的路径与网络。真正 worker sandbox、网络策略、资源配额未完成；不能向不可信多租户开放此能力 |

## 推荐后续顺序与验收

1. **独立受限 worker**：明确文件挂载范围、服务凭据不入容器、网络出站策略和进程/内存/磁盘限额。必须实际验证逃逸路径、超时后子进程、读外部路径和跨用户访问；Dockerfile 能构建不代表隔离完成。
2. **完整变更事务/检查点**：多文件副作用前持久计划，落盘进度可恢复；工作区快照明确排除规则、配额、保留期和人工修改冲突。验收应在各落盘阶段杀进程，证明重启后不会静默丢文件或重复执行。
3. **状态与协议契约收敛**：从同一 ToolSpec 生成 schema/manifest/路由，逐步给聊天输入/事件状态增加类型约束；保留兼容协议的固定故障样本，覆盖空文件、分片参数、重复 call ID、半包和供应商限流。
4. **长时间运行与运维**：增加运行耗时/队列等待/取消回收/持久失败指标，测试数据增长、磁盘耗尽、SQLite 锁竞争和备份恢复；建立版本升级、回退和数据迁移的演练记录。

这些工作应按可复现问题逐项推进。没有必要为追赶功能列表重写整个前后端；应要求每项新能力同时交付所有权规则、失败语义、恢复方式和可重复的验收证据。

夜间收尾补充：会话删除现与前台/后台准入互斥，并等待取消执行器清理；已删除会话不能恢复。后台 AI 尚无前台式持久检查点，因此服务重启将中断任务标记为需要核验的失败，保留取消请求，避免自动重复副作用。匹配数据库与用户文件的小规模离线备份已在独立端口恢复并核验；大型/WAL/崩溃场景仍未覆盖。

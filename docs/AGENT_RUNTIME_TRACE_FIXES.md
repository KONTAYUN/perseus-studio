# 后台运行记录缺口修复

日期：2026-10-22。对应 [运行验收](AGENT_RUNTIME_ACCEPTANCE_RESULTS.md) 中 AR-01、AR-02。两项已修复，完成隔离故障回归和更新后的真实 Grok 复测。

## 原因与修改

### AR-01：后台转交走独立完成分支

原先普通回复会保存 `run_completed`，转交后台任务只更新成功状态、保存创建回执。这使数据库状态与可回放的事件轨迹不一致；普通路径还存在先成功、后写完成事件的间隙。

现在普通完成和后台转交共用 [CompleteRun](../backend/internal/store/run_completion.go)：成功状态、完成事件和检查点清理在同一数据库事务中提交。事件写入失败时整体回滚，保留恢复检查点；不会先留下 succeeded 快照。相同已完成身份重放返回已有事件，不重复追加。

转交另外保存 `task_handoff_completed`，`run_completed` 包含 `task_id`、`completion_kind=background_task_handoff`、`outcome=success`。这里的 success 表示前台转交成功，后台目标完成情况由其自身 outcome 表示。创建回执正文及 metadata 都包含任务身份，任务 payload 反向保存 `source_run_id`。

涉及 [普通完成](../backend/internal/api/conversation_run_completion.go)、[后台转交](../backend/internal/api/conversation_run_task_completion.go)、[共用发布入口](../backend/internal/api/conversation_run_terminal.go)。

### AR-02：后台回调丢弃大部分事件和正文

原先后台事件只转换为少数简化日志，模型中间文字、请求尝试/重试、恢复等结构化数据丢失；返回工具调用时的说明也会被显示策略暂缓，不能仅接显示回调。此外后台正文流原本被关闭，流中断时更难保留片段。

现在通过 [taskExecutionTrace](../backend/internal/tasks/manager_ai_trace.go) 保存任务事件：

- 新增 `task_events`，按 task_id 连续编号，execution_id 区分初次执行和产物修复轮次；原有前台 `agent_events` 不混入后台轨迹。
- 保留模型完整返回文字 `model_response_observed`，并保留可观察正文增量、消息完成/撤回、完成预览及丢弃事件；暂缓展示的工具说明仍可审阅，界面显示策略保留。
- 完整保存请求尝试/重试、工具生命周期、完成拒绝、恢复、循环保护和预算等已有结构化事件；完整工具参数/结果继续关联原 `tool_calls` 台账。
- 某条事件保存失败即取消该次执行，返回原持久化错误并阻止后续工具副作用；不会把内部保存失败当成用户主动取消。
- 正常结束、失败、取消，以及服务重启后的中断处理均有明确任务终态事件；任务状态和终态事件同一事务提交，重复终止不会覆盖已完成结果。
- 最终聊天消息含 task_id、source_run_id、outcome；任务结果保存最终消息身份、outcome、未完成原因、预算、修复次数及产物校验结果。

顺带修正后台完成语义：模型诚实返回 partial/blocked 或预算未完成时，保留该结果，不强迫其反复补做；task.status=done 表示执行结束，result.ok 仅在结构化 success 且产物校验通过时为 true。存在未完成事项时任务进度明确提示查看最终回复。

数据入口：`GET /api/tasks/{task_id}/events?after_seq=0&limit=200`，返回 items 和 next_seq；升序分页，limit 1–1000。核对任务所有者，其他用户返回 404；非法游标返回 400。刷新后可从原游标补读，超过 200 条不会悄悄只给最后一段。

实现见 [task_events.go](../backend/internal/store/task_events.go)、[任务回放 API](../backend/internal/api/task_events.go)、[后台执行](../backend/internal/tasks/manager_ai.go)、[重启核对](../backend/internal/store/task_recovery.go)。本轮没有新增任务时间线界面，中间片段保存到任务事件；聊天历史仍只加入后台最终结果。

## 验证

新增 10 个后端顶层回归和 3 个浏览器用例，全部使用独立数据库/用户目录及本地确定性供应商。

| 验证 | 结果 |
| --- | --- |
| 完成事务 | 注入 `run_completed` 写入失败：成功状态不提交、检查点保留；成功后检查点清除；重复完成不重复事件，用户隔离通过 |
| 后台转交 | 正常路径任务身份/完成事件一致；完成事件拒写时前台失败，实际已创建任务保留，不谎称副作用回滚 |
| 中间回复/重试/partial | 先 502 重试，再缺失文件报错、读取现有文件、诚实 partial；所有文字与错误可审，保留 partial/校验失败，零额外修复轮次 |
| 流式正文 | SSE 说明、完整返回文字及完成预览均保留；没有把流式分片当成多次模型请求 |
| 修复轮次 | 初次缺产物后补做成功，两个独立 execution_id、repair_rounds=1，最终 success/ok=true |
| 保存记录失败 | 首个写入成功后注入工具完成事件保存失败；首个成果保留，第二次写入未发生，任务失败保留原错误原因 |
| 取消 | 供应商请求在途取消后稳定 canceled，请求开始、执行失败/取消原因和任务终态都保留 |
| 长记录/所有权 | 205 条记录按 200+5 分页，无重复遗漏；其他用户读写被拒；任务终态失败回滚、迟到取消不改已完成历史 |
| 重启 | 中断任务的终态轨迹与状态一起提交；故障时回滚日志及状态，再次恢复不重复追加 |
| 最终 Go 全量 | `go test ./... -count=1 -timeout 180s` 全部通过 |
| 静态检查 | `go vet ./...`、结构门禁通过 |
| 最终 Chrome 全套 | 36 通过、1 个 opt-in 真实供应商用例跳过；新增成功/partial/取消三条均通过，覆盖刷新及小页回放 |

回归源文件：[store](../backend/internal/store/runtime_trace_test.go)、[API](../backend/internal/api/task_trace_test.go)、[后台执行](../backend/internal/tasks/manager_ai_trace_test.go)、[浏览器](../tests/e2e/task-trace.spec.js)。

日志：`_local_artifacts/task-trace-go-final.log`、`task-trace-vet-verified.log`、`task-trace-browser-all.log`、`task-trace-background-verified.log`。首轮失败也保留：新增用例暴露暂缓显示文字和后台关闭正文流的问题，修正后重跑通过；取消用例的测试服务器退出方式另已修正，不混记为产品故障。

## 8080 真实复测与数据保护

更新前确认活动运行、后台任务、待发输入及调度均为零；SQLite backup 备份数据库并复制用户目录。切换前后原有 **60 个文件**摘要和 users/model_providers/search_providers/skills/conversations/messages/tasks/agent_runs **八类表全部原有行**摘要一致。

8080 现运行 `_local_artifacts/perseus-task-trace-verified.exe`，PID 76648，SHA-256 `b3540a9e828f12d0623f8a5aa6fbe385e040a340fcbe0e01e5f68889a6a337fc`，复用原前端、数据库和用户目录。

随后在新建 `验收测试/后台追踪修复-20261022-1925/` 运行真实 Grok 任务，未改供应商或默认配置：

- 前台 2 次可观察请求，正确加载任务工具并创建后台任务；恰好 1 条 `run_completed`，任务身份完整。
- 后台 4 轮模型返回、4 次可观察请求、0 次重试、3 次工具：读原稿、写报告、回读报告；共 57 条任务事件。
- 四轮中间文字、最终回复、前台两段说明及平台回执共 8 个完整片段逐条审读，声明与事件/落盘内容一致，没有绕圈或虚报保存。
- 报告是非空 Markdown 对照表，一层/36 人/免费及三项待确认均正确；原稿逐字未变。刷新后完整回放一致，浏览器无 pageerror。
- 最终原有文件和记录再次核对无变化；仅本次测试目录新增 2 个文件，数据库完整性/外键检查通过，运行/后台/待发均空闲。隔离 18083 实例检查空闲后停止。

真实运行：`run_1792668321004332600_3`；任务：`task_1792668334372281800_45`。

证据和备份在 `_local_artifacts/task-trace-fix/20261022/`：`live-evidence.json`、[逐条审阅](../_local_artifacts/task-trace-fix/20261022/live-review.md)、`summary.json`、`after-switch.json`、`final-integrity.json`、`backup/app.before-trace-fix.db`。私有原件留本地，不输出凭据。

## 边界

历史任务已经丢弃的文字无法补回，本轮修复从新版运行开始生效。前台终态事务覆盖状态、完成事件和检查点；创建后台任务、创建聊天回执仍是此前的独立操作，本轮没有宣称实现跨任务转交的全事务/崩溃幂等。数据库完全不可写时无法保证把失败终态也写入；执行会停止，并通过进程日志及重启核对保留调查路径。

任务事件增加了持久存储量，尚未实现独立归档策略。没有进行 Linux 云端部署、长时间压力或强杀断电的真实运行测试，不把本次回归描述为零缺陷保证。

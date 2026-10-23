# Perseus Studio

[English](README.md) | 简体中文

Perseus Studio 是一个浏览器里的轻量 AI 工作区，集成了文件编辑、AI 对话、后台任务执行和 Skill 系统。项目支持多用户和按用户隔离的数据目录，并提供单容器 Docker 部署方式。

部分内部包名仍保留历史上的 `webide-*` 命名，以兼容现有 Go module、导入路径和锁文件。面向用户的产品文档和界面统一使用 **Perseus Studio**。

## 功能

- **多用户系统**：注册、登录、按用户隔离数据
- **管理员面板**：用户管理、注册开关、角色管理
- **文件编辑器**：标签页、语法高亮、拖拽上传
- **AI 对话**：支持 OpenAI 和 Anthropic 协议，以及工具调用
- **后台任务**：通过 SSE 实时推送任务状态
- **Skill 系统**：支持 zip 上传，或从聊天附件安装
- **网页搜索/提取**：支持 Tavily 或 Brave provider
- **工作区文件管理**：按用户目录隔离，删除进入 `回收站`，补丁有持久恢复计划；宿主命令默认关闭

## 快速开始

```bash
cp .env.example .env
docker compose up -d --build
# 打开 http://localhost:8080
# 第一个注册用户会自动成为管理员。
```

## 执行边界

命令执行需要管理员显式设置 `PERSEUS_COMMAND_EXECUTION=trusted_host` 并重启服务。启用后，只有管理员的 Agent 可调用命令，但进程拥有服务账号的操作系统文件和网络权限；这不是 OS 沙箱。不可信多用户命令执行仍需独立受限 worker。默认关闭不影响文件读取、结构化补丁或编辑器使用。

文件修改只从结构化工具调用执行，聊天正文、Markdown 围栏中的补丁示例只展示。模型高级设置可以明确选择系统消息布局（默认合并为一条，或使用多条）；显示名称不再决定运行预算，实际上下文长度应按供应商配置。

任务结果区分成功、部分完成和受阻；它与运行请求是否正常结束分开记录。失败验证命令不会被后续无关读文件操作清除。每轮模型请求含重试/回退共享总预算：前台默认 192 次、20 分钟，后台 384 次、2 小时；主动恢复为新一轮预算，这不是费用上限。补丁崩溃恢复保留原文并核对哈希，遇到后续人工编辑则停止，不能视为 SQLite 与文件系统的 ACID 事务。当前验证和剩余限制见 [设计审计整改报告](docs/DESIGN_AUDIT_REMEDIATION.md)。

## Docker 镜像

每次推送到 `master` 后，GitHub Actions 会自动构建镜像并发布到 GitHub Container Registry：

```text
ghcr.io/kontayun/perseus-studio:latest
ghcr.io/kontayun/perseus-studio:<commit-sha>
```

如果镜像包是私有的，服务器需要先用带 `read:packages` 权限的 GitHub token 登录：

```bash
docker login ghcr.io
docker pull ghcr.io/kontayun/perseus-studio:latest
```

服务器上可以使用基于 image 的 Compose 文件，之后手动更新：

```bash
docker compose pull
docker compose up -d
```

## 用户系统

- **第一个注册用户** 会自动成为 `admin`。
- 管理员可以在 Settings -> User Management 中打开或关闭注册。
- 管理员可以 **创建、禁用、启用、重置密码、提升/降低角色、删除** 用户。
- **删除用户会永久删除** 该账号以及 `/data/users/<user_id>/` 下的全部数据，此操作不可恢复。
- 每个用户都有独立的工作区、上传文件、Skills、会话、任务、模型 provider 和搜索 provider。
- 当系统里没有任何用户时，注册始终开放，以避免锁死。

## 本地开发

### 前端

```bash
npm install
npm run dev          # Vite dev server on :5173
```

### 后端

```bash
cd backend
go run ./cmd/server  # 监听 :8080
```

设置 `FRONTEND_DIST=../dist`，并按 `.env.example` 配置其他环境变量。

### 测试

```bash
npm run test:ui
npm run test:unit
npm run build
cd backend && go test ./...

# 启动 Docker 后可选的完整本地冒烟测试
npm run test:e2e
```

## 数据持久化

所有运行时数据默认存放在 `./data/`，Docker 中挂载为 `/data`：

| 路径 | 内容 |
|---|---|
| `data/app.db` | SQLite 数据库：用户、任务、会话、配置、Skills |
| `data/users/<user_id>/workspace/` | 每个用户的工作区文件 |
| `data/users/<user_id>/uploads/` | 每个用户的上传文件 |
| `data/users/<user_id>/skills/` | 每个用户安装的 Skill 包 |

旧路径 `data/workspace/`、`data/uploads/`、`data/skills/` 不再用于新安装。

## 文档

- [ARCHITECTURE.md](ARCHITECTURE.md)：模块布局、关键流程、数据模型
- [DEPLOY.md](DEPLOY.md)：Docker 部署、配置、反向代理、安全说明
- [HARNESS.md](HARNESS.md)：Agent 行为规则，包括工作区、工具、上下文、任务、诊断和前端 UI 治理
- [docs/UI_DESIGN_SYSTEM.md](docs/UI_DESIGN_SYSTEM.md)：前端 UI 标准，包括按钮、输入、面板、动效、颜色和共享组件
- [docs/BACKUP.md](docs/BACKUP.md)：备份与恢复清单

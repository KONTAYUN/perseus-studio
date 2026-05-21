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
- **工作区沙箱**：按用户隔离，删除进入软删除目录 `回收站`

## 快速开始

```bash
cp .env.example .env
docker compose up -d --build
# 打开 http://localhost:8080
# 第一个注册用户会自动成为管理员。
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

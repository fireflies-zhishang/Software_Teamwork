# 电力行业知识管理系统

本仓库用于开发一个面向电力行业的知识管理系统。系统目标是沉淀行业文档、文件资料和结构化知识，并提供知识检索、智能问答和文档生成能力。

当前仓库处于架构与工程规范建设阶段。本文档描述目标技术栈、目标微服务划分、推荐目录结构和 CI/CD 约定；具体服务代码会按该结构逐步落地。

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 前端 | React + TypeScript |
| 后端 | Go 微服务 |
| 服务通信 | HTTP/REST |
| 关系数据库 | PostgreSQL |
| 缓存 / 队列 | Redis |
| 向量数据库 | Qdrant |
| 对象存储 | MinIO |
| 本地与单机部署 | Docker Compose |
| CI/CD | GitHub Actions |
| 仓库结构 | Monorepo |

前端当前落地在 `apps/web/`，使用 Bun + Vite。后续如果切换构建工具，再同步更新启动命令和 CI 细节。

## 系统架构

系统采用以网关为入口的微服务架构：

```text
frontend
   |
   v
gateway service
   |
   +--> auth service
   +--> file service
   +--> 智能问答
   +--> 知识库
   +--> 文档生成

基础设施:
postgres + redis + qdrant + minio
```

服务职责：

| 服务 | 职责 |
| --- | --- |
| `frontend` | 面向用户的 React + TypeScript 应用，通过网关访问后端能力。 |
| `gateway` | 后端统一入口，负责路由、鉴权上下文传递、聚合接口和跨服务请求协调。 |
| `auth` | 用户身份、登录认证、权限控制、令牌或会话管理。 |
| `file` | 文件上传、文件元数据、对象存储协调，以及文件处理流程入口。 |
| `qa` | 智能问答服务，基于知识库检索结果生成回答。 |
| `knowledge` | 知识导入、切分、索引、元数据管理和检索协调。 |
| `document` | 报告、材料、知识摘要等文档生成流程。 |

基础设施职责：

| 组件 | 用途 |
| --- | --- |
| PostgreSQL | 业务数据、用户数据、文件元数据、知识元数据。 |
| Redis | 缓存、会话、短期任务状态或轻量队列。 |
| Qdrant | 向量索引和相似度检索。 |
| MinIO | 原始文件、生成文档和其他对象数据。 |

## 目标目录结构

```text
.
├── apps/
│   └── web/
├── services/
│   ├── gateway/
│   │   └── go.mod
│   ├── auth/
│   │   └── go.mod
│   ├── file/
│   │   └── go.mod
│   ├── qa/
│   │   └── go.mod
│   ├── knowledge/
│   │   └── go.mod
│   └── document/
│       └── go.mod
├── deploy/
│   └── docker-compose.yml
├── docs/
├── .github/
│   └── workflows/
└── .trellis/
    ├── spec/
    └── tasks/
```

每个 Go 微服务维护独立的 `go.mod`，作为独立的构建、测试和镜像发布单元。除非后续明确引入共享库，否则不要默认跨服务共享 Go 包。

## 本地开发

当前 README 先定义目标启动方式。服务代码和 `deploy/docker-compose.yml` 落地后，本地开发应遵循以下流程：

```bash
git clone https://github.com/Sakayori-Iroha-168/Software_Teamwork.git
cd Software_Teamwork

docker compose -f deploy/docker-compose.yml up -d postgres redis qdrant minio
```

启动前端：

```bash
cd apps/web
bun install
bun run dev
```

启动单个 Go 服务：

```bash
cd services/gateway
go mod download
go run ./cmd/server
```

运行质量检查：

```bash
# 前端
cd apps/web
bun run lint
bun run test
bun run build

# Go 服务示例
cd services/gateway
go test ./...
go build ./cmd/server
```

## CI/CD 约定

GitHub Actions 应按服务路径拆分检查，避免一个服务的小改动触发所有服务的完整构建。

推荐流水线：

| 阶段 | 触发范围 | 内容 |
| --- | --- | --- |
| PR Guard | 所有 PR | 检查 PR 目标分支、fork 协作规则和分支同步状态。 |
| Commitlint | 所有 PR | 检查 Conventional Commits。 |
| Frontend CI | `apps/web/**` | 安装依赖、lint、test、build。 |
| Go Service CI | `services/<service>/**` | 对变更服务执行 `go test ./...` 和 `go build`。 |
| Docker Build | 服务或部署文件变更 | 构建对应服务镜像。 |
| Deploy | 合并到部署分支后 | 在单机环境通过 Docker Compose 拉起最新服务。 |

部署目标是单机 Docker Compose。生产部署需要至少提供：

- `deploy/docker-compose.yml`
- 服务镜像构建规则
- 环境变量模板
- 数据库、Redis、Qdrant、MinIO 的持久化卷配置
- 部署失败时的回滚说明

## 协作规范

本仓库采用 fork + PR 的协作方式：

- 从主仓库最新 `develop` 创建个人分支。
- 所有日常开发 PR 指向 `develop`。
- 禁止直接向 `develop` 或 `main` push 功能、修复或文档修改。
- `main` 只用于发布合并。

完整流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## Commit 规范

所有 commit 必须遵循 Conventional Commits：

```text
<type>(<scope>): <subject>
```

示例：

```text
feat(gateway): add health check endpoint
fix(auth): handle expired token
docs(readme): describe service architecture
chore(ci): add go service workflow
```

完整规则见 [.trellis/spec/guides/commit-convention.md](.trellis/spec/guides/commit-convention.md)。

## 工程规范

后续实现应遵循 Trellis spec：

- 后端规范：[.trellis/spec/backend/](.trellis/spec/backend/)
- 前端规范：[.trellis/spec/frontend/](.trellis/spec/frontend/)
- CI/CD 规范：[.trellis/spec/cicd.md](.trellis/spec/cicd.md)
- 共享思考指南：[.trellis/spec/guides/](.trellis/spec/guides/)

# 前端协作工作流

本文档定义本仓库的前端开发补充规则，面向团队成员阅读。

仓库级分支、PR、提交和合并策略以 [`CONTRIBUTING.md`](../CONTRIBUTING.md) 为唯一权威来源。本文只补充前端目录、包管理器、检查命令和前端 CI 建议；如果本文和 `CONTRIBUTING.md` 冲突，以 `CONTRIBUTING.md` 为准。

## 远端仓库

前端开发沿用 `CONTRIBUTING.md` 中的 remote 约定：

```txt
upstream = https://github.com/Sakayori-Iroha-168/Software_Teamwork
origin   = https://github.com/L-1ngg/Software_Teamwork
```

`upstream` 是团队主仓库，`origin` 是个人 fork 仓库。

## 分支和 PR 约定

默认按 `CONTRIBUTING.md` 走 fork + PR 流程：

```txt
upstream/main     发布分支，不做日常开发 PR
upstream/develop  主仓库日常集成分支
origin/*          个人 fork 分支
```

## 日常开发流程

从最新的 `upstream/develop` 创建前端功能分支：

```bash
git fetch upstream --prune
git switch -c L1nggTeam/feat/<short-name> upstream/develop
```

个人开发分支推送到自己的 fork：

```bash
git push -u origin L1nggTeam/feat/<short-name>
```

PR 方向按 `CONTRIBUTING.md` 固定为：

```txt
from: L-1ngg/Software_Teamwork:L1nggTeam/feat/<short-name>
to:   Sakayori-Iroha-168/Software_Teamwork:develop
```

如果开发期间主仓库 `develop` 已经更新：

```bash
git fetch upstream --prune
git rebase upstream/develop
git push --force-with-lease
```

`--force-with-lease` 只允许用于个人 fork 上的功能分支，不要用于团队共享分支。

## 前端目录

前端源码统一放在：

```txt
apps/web/src/
```

前端应用根目录是：

```txt
apps/web/
```

这样可以在同一个仓库中清晰分离前端和后端代码。

## 包管理器

前端统一使用 Bun。

规则：

- 使用 `bun install` 安装依赖。
- 从仓库根目录使用 `bun run --cwd apps/web <script>` 执行前端脚本。
- 不提交 `package-lock.json`、`yarn.lock` 或 `pnpm-lock.yaml`。
- 提交当前项目 Bun 版本生成的 Bun lockfile。
- 创建第一个前端 package 后，在根目录 `package.json` 的 `packageManager` 字段固定 Bun 版本。

当根目录存在 `package.json` 时，建议提供这些脚本：

```json
{
  "scripts": {
    "dev:web": "bun run --cwd apps/web dev",
    "build:web": "bun run --cwd apps/web build",
    "lint:web": "bun run --cwd apps/web lint",
    "typecheck:web": "bun run --cwd apps/web typecheck",
    "check:web": "bun run --cwd apps/web check"
  }
}
```

## 前端必跑检查

提交前端 PR 前，至少运行：

```bash
bun run --cwd apps/web check
bun run --cwd apps/web build
```

其中 `check` 应包含：

```txt
typecheck
lint
format:check
```

## Lint 和格式化基线

前端应用使用这套基线：

```txt
ESLint Flat Config
Prettier
TypeScript strict
eslint-plugin-react
eslint-plugin-react-hooks
eslint-plugin-jsx-a11y
eslint-plugin-simple-import-sort
eslint-config-prettier
```

推荐规则强度：

- React Hooks 规则错误直接阻断。
- `exhaustive-deps` 初期设为 warning。
- 强制使用 type-only imports。
- 强制 import/export 排序。
- `any` 初期设为 warning，代码稳定后升为 error。
- `console` 设为 warning，允许 `console.warn` 和 `console.error`。

## 提交信息规范

使用 Conventional Commits：

```txt
feat(knowledge): add document upload dialog
feat(qa): support sse streaming answer
feat(report): add outline editor
fix(auth): persist session after refresh
chore(lint): setup eslint prettier husky
docs(frontend): document workflow
```

推荐接入：

```txt
Husky
lint-staged
Commitlint
```

推荐 pre-commit 行为：

```sh
bunx lint-staged
bun run --cwd apps/web typecheck
```

推荐 commit-msg hook：

```sh
bunx commitlint --edit "$1"
```

## PR 要求

前端 PR 合入主仓库 `develop` 前应满足：

- PR 目标分支是 `develop`。
- 代码来自个人 fork 的功能分支。
- `bun run --cwd apps/web check` 通过。
- `bun run --cwd apps/web build` 通过。
- 至少一名成员 review 通过。
- PR 描述列出用户可见变化、验证命令和已知风险。

## CI

前端 CI 应在以下场景运行：

```txt
pull_request -> develop
push         -> develop
```

推荐 CI 命令：

```bash
bun install --frozen-lockfile
bun run --cwd apps/web check
bun run --cwd apps/web build
```

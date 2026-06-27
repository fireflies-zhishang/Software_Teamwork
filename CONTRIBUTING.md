# 协作规范

本仓库采用 fork + PR 的协作方式。所有代码、文档和配置修改默认都必须通过
Pull Request 合入主仓库的 `develop` 分支。

## 基本原则

- 主仓库日常集成分支是 `develop`。
- 禁止向 `main` 发起日常开发 PR。
- 禁止直接向 `develop` 或 `main` push 功能、修复或文档修改。
- 必须从主仓库最新 `develop` 创建个人 fork 中的独立分支。
- PR 必须指向主仓库 `develop`。
- `main` 只用于发布，由维护者从 `develop` 做发布合并。

## 小组 Label

PR 会根据提交账号和修改文件路径自动添加匹配 label。账号规则会匹配 PR
发起人以及 PR commit 的 GitHub author/committer login 或数字 ID。配置见
[.github/labeler.json](.github/labeler.json)。如果匹配到的 label 在仓库中不存在，
Auto Label workflow 会跳过该 label 并在日志中提示。

## 分支规则

推荐分支命名格式：

```text
<team>/<type>/<short-description>
```

示例：

```text
L1nggTeam/feat/login-page
PrimeTeam/fix/user-api-null
JerryTeam/docs/contributing-guide
```

`type` 建议与 commit type 保持一致：

- `feat`
- `fix`
- `docs`
- `style`
- `refactor`
- `test`
- `chore`
- `perf`

## Commit 规范

所有 commit 必须遵循 Conventional Commits：

```text
<type>(<scope>): <subject>
```

允许的 `type`：

| Type | 用途 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档 |
| `style` | 代码格式，不改变逻辑 |
| `refactor` | 重构，不新增功能也不修复 bug |
| `test` | 测试 |
| `chore` | 构建、依赖、工具、CI/CD |
| `perf` | 性能优化 |
| `revert` | 回滚 |

好例子：

```text
feat(frontend): add login form
fix(backend): handle empty user response
docs(workflow): add fork pr rules
chore(ci): add pr guard workflow
```

坏例子：

```text
update
fix bug
wip
changes
final
```

完整规则见 [.trellis/spec/guides/commit-convention.md](.trellis/spec/guides/commit-convention.md)。
AI agent 在运行 `git commit` 前必须阅读本文档和
[.agents/git-commit-checklist.md](.agents/git-commit-checklist.md)。

## 标准开发流程

1. Fork 主仓库到个人账号。
2. 本地配置 `upstream` 指向主仓库，`origin` 指向个人 fork。
3. 拉取主仓库最新 `develop`。
4. 从 `upstream/develop` 创建个人工作分支。
5. 在个人分支提交修改。
6. 推送到个人 fork。
7. 使用 `gh pr create` 向主仓库 `develop` 发起 PR。
8. 等待 CI、PR Guard、Commitlint 和 review 通过后合并。

### 1. 登录 GitHub CLI

```bash
gh auth login
gh auth status
```

### 2. Fork 主仓库

```bash
gh repo fork Sakayori-Iroha-168/Software_Teamwork --remote --clone=false
```

如果已经 fork 过，可以跳过这一步。

### 3. 配置 Remote

确认 remote：

```bash
git remote -v
```

推荐配置：

```bash
git remote set-url origin git@github.com:YOUR_NAME/Software_Teamwork.git
git remote add upstream git@github.com:Sakayori-Iroha-168/Software_Teamwork.git
```

如果 `upstream` 已存在：

```bash
git remote set-url upstream git@github.com:Sakayori-Iroha-168/Software_Teamwork.git
```

最终应满足：

```text
origin    -> 你的个人 fork
upstream  -> 主仓库
```

### 4. 从最新 develop 创建分支

```bash
git fetch upstream
git switch -c L1nggTeam/feat/login-page upstream/develop
```

不要从 `main`、本地旧分支或主仓库临时分支创建开发分支。

### 5. 提交修改

```bash
git status
git add .
git commit -m "feat(frontend): add login page"
```

Commit message 必须遵循上文。

### 6. 推送到个人 fork

```bash
git push -u origin L1nggTeam/feat/login-page
```

### 7. 创建 PR 到主仓库 develop

```bash
gh pr create \
  --repo Sakayori-Iroha-168/Software_Teamwork \
  --base develop \
  --head YOUR_NAME:L1nggTeam/feat/login-page \
  --title "feat(frontend): add login page" \
  --body-file .github/pull_request_template.md
```

注意：

- `--base` 必须是 `develop`，不能是 `main`。
- `--head` 必须是 `YOUR_NAME:<branch>`，也就是个人 fork 中的分支。

### 8. PR 前同步最新 develop

如果主仓库 `develop` 更新了，需要 rebase：

```bash
git fetch upstream
git rebase upstream/develop
git push --force-with-lease
```

禁止使用普通 `--force`。只使用 `--force-with-lease`。

### 9. 查看 PR 状态

```bash
gh pr status
gh pr checks <PR_NUMBER> --repo Sakayori-Iroha-168/Software_Teamwork
gh pr view --web
```

## PR 前检查

提交 PR 前必须确认：

- 当前分支来自最新 `upstream/develop`。
- PR base 是 `develop`，不是 `main`。
- PR head 是个人 fork 的分支。
- Commit message 符合 Conventional Commits。
- 已运行相关 lint、type-check、test 或 build。

## 合并要求

合并 PR 前必须满足：

- CI 通过。
- PR Guard 通过。
- Commitlint 通过。
- 至少一名维护者 review 通过。
- 分支包含当前最新 `develop`。

## 常见错误

### PR 目标分支选成 main

关闭该 PR，重新向 `develop` 发起 PR：

```bash
gh pr close <PR_NUMBER> --repo Sakayori-Iroha-168/Software_Teamwork
gh pr create --repo Sakayori-Iroha-168/Software_Teamwork --base develop
```

### 分支落后 develop

```bash
git fetch upstream
git rebase upstream/develop
git push --force-with-lease
```

### Commit message 不规范

修改最近一次 commit：

```bash
git commit --amend -m "fix(backend): handle empty user response"
git push --force-with-lease
```

修改多个 commit：

```bash
git fetch upstream
git rebase -i upstream/develop
git push --force-with-lease
```

## 维护者职责

- 不合并目标分支不是 `develop` 的日常开发 PR。
- 不合并来自主仓库分支的成员 PR。
- 发现 commit 不规范时要求 contributor rebase 或 squash 后重提。
- 发布时由维护者将 `develop` 合并到 `main`。

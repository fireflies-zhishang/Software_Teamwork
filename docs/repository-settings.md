# 仓库维护设置

本文档面向仓库维护者，记录需要在 GitHub 远端仓库配置的保护规则。文档和
workflow 只能拦截 PR 行为；分支保护和 label 仍需要维护者在 GitHub 仓库中
配置。

## 可选 Label

```text
L1nggTeam
PrimeTeam
JerryTeam
frontend
backend
documentation
ci
deployment
trellis
service:gateway
service:auth
service:file
service:qa
service:knowledge
service:document
```

检查现有 label：

```bash
gh label list --repo Sakayori-Iroha-168/Software_Teamwork
```

## develop 分支保护

`develop` 是日常集成分支，必须通过 PR 合入。

推荐在 GitHub 页面配置：

- Require a pull request before merging
- Require approvals，至少 1 人
- Require status checks to pass before merging
- Require branches to be up to date before merging
- Include administrators，按团队管理要求决定是否启用
- Restrict who can push to matching branches，禁止普通成员直接 push

建议作为 required checks 的 workflow：

- `PR Guard`
- `Commitlint`
- `CI`

如果使用 `gh api` 配置，需要先确保对应 checks 至少运行过一次，再按实际
check 名称补入 `contexts`。

`Auto Label` 负责根据提交账号和修改路径给 PR 自动添加 label。它不是合并
门禁，不建议加入 required checks。

## main 分支保护

`main` 不是日常开发 PR 目标。普通开发和文档修改只能 PR 到 `develop`。

推荐规则：

- Restrict who can push to matching branches，仅允许发布维护者或发布机器人更新
- Require status checks to pass before updating
- Require linear history
- 禁止普通成员直接 push

发布方式由维护者执行，可以是受限账号将 `develop` 合并到 `main`，也可以是
后续增加专门的 release workflow。不要让普通成员向 `main` 发起日常开发 PR。

## PR Guard 规则

[.github/workflows/pr-guard.yml](../.github/workflows/pr-guard.yml) 会检查：

- PR base 必须是 `develop`
- PR head 必须来自个人 fork
- PR 分支必须包含当前最新 `develop`

## Auto Label 规则

[.github/workflows/auto-label.yml](../.github/workflows/auto-label.yml) 会读取
[.github/labeler.json](../.github/labeler.json)，并按两类规则给 PR 添加 label：

- `accountLabels`: GitHub 账号 login 或数字 ID 到 label 的映射，匹配 PR
  发起人以及 PR commit 的 GitHub author/committer
- `pathLabels`: 文件路径 glob 到 label 的映射

示例：

```json
{
  "accountLabels": [
    {
      "accounts": ["username", "12345678"],
      "labels": ["L1nggTeam"]
    }
  ],
  "pathLabels": [
    {
      "paths": ["apps/web/**"],
      "labels": ["frontend"]
    }
  ]
}
```

workflow 只添加仓库中已经存在的 label。新增小组或领域 label 后，需要先在
GitHub 仓库创建 label，再更新 `.github/labeler.json`。

## Commitlint 规则

[.github/workflows/commitlint.yml](../.github/workflows/commitlint.yml) 会检查每个
PR commit 的第一行是否符合 Conventional Commits：

```text
<type>(<scope>): <subject>
```

允许的 type：

```text
feat, fix, docs, style, refactor, test, chore, perf, revert
```

## 维护者日常检查

```bash
gh pr list --repo Sakayori-Iroha-168/Software_Teamwork --base develop
gh pr checks <PR_NUMBER> --repo Sakayori-Iroha-168/Software_Teamwork
gh pr view <PR_NUMBER> --repo Sakayori-Iroha-168/Software_Teamwork --json baseRefName,headRepositoryOwner,headRefName,labels
```

发现 PR 目标分支、来源仓库或 commit 不符合规范时，不要合并。要求
contributor 修正后再 review。

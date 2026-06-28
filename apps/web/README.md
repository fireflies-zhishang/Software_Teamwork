# 前端应用

前端应用位于 `apps/web`，源码位于 `apps/web/src`。

## 常用命令

从仓库根目录运行：

```bash
bun run --cwd apps/web dev
bun run --cwd apps/web check
bun run --cwd apps/web build
```

或进入 `apps/web` 后运行：

```bash
bun run dev
bun run check
bun run build
```

## 当前骨架

- `src/app`：应用入口、Provider、Query Client、路由占位。
- `src/layouts`：基础 AppShell。
- `src/pages`：页面级组合入口。
- `src/features`：后续按业务模块沉淀功能组件、hooks、schema。
- `src/components`：跨模块通用组件区域。
- `src/api/generated`：后续由 OpenAPI 生成的 API 类型和客户端。

## 质量基线

前端使用 Bun、Vite、React、TypeScript、ESLint Flat Config 和 Prettier。提交前至少运行：

```bash
bun run check
bun run build
```

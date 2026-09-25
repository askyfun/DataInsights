# 贡献指南

完整的贡献流程、语言级编码规范与提交前门禁见 **[`docs/developer-guide/contributing.md`](docs/developer-guide/contributing.md)**。本文件只列最短须知，作为入口。

## 三条硬约束

- **包管理器只用 pnpm** —— 根目录与 `frontend/` 的 `package.json` 都带 `preinstall` 钩子，npm / yarn / bun 会被直接拒绝执行（`packageManager` 钉死版本，`.npmrc` 开启 `package-manager-strict`）。
- **提交前必跑门禁**：后端 `go test -race ./...`、前端 `pnpm build:check`。pre-commit 钩子会对暂存的 `.ts/.tsx` 执行 `biome check`。
- **接口契约的唯一事实源是 [`api/openapi.yaml`](api/openapi.yaml)** —— 改接口先改它，再跑 `make api-gen` 同步 Go 与 TypeScript 双端类型（生成器与 `typescript@7` 的已知不兼容见 [`docs/developer-guide/troubleshooting.md`](docs/developer-guide/troubleshooting.md)）。

## 文档在哪

| 想了解 | 看 |
|---|---|
| 本地环境搭建与运行命令 | [`docs/developer-guide/dev-setup.md`](docs/developer-guide/dev-setup.md) |
| 架构与关键设计决策（含「为什么这样设计」） | [`docs/developer-guide/architecture.md`](docs/developer-guide/architecture.md) |
| 排障（外部开发者也会踩的坑） | [`docs/developer-guide/troubleshooting.md`](docs/developer-guide/troubleshooting.md) |
| 活跃待办与数据源方言能力矩阵 | [`docs/developer-guide/backlog.md`](docs/developer-guide/backlog.md) |
| 前端表面系统规范 | [`docs/developer-guide/design-system.md`](docs/developer-guide/design-system.md) |
| 图表查询链路设计 | [`docs/developer-guide/chart-query-design.md`](docs/developer-guide/chart-query-design.md) |

## 许可

以仓库根的 [`LICENSE`](LICENSE) 为准。提交即表示你同意按该许可分发你的贡献。

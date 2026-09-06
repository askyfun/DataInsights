# Cerebrum

> OpenWolf's learning memory. Updated automatically as the AI learns from interactions.
> Do not edit manually unless correcting an error.
> Last updated: 2026-05-05

## User Preferences

<!-- How the user likes things done. Code style, tools, patterns, communication. -->

## Key Learnings

- **Project:** data-insights
- **Description:** 拖拽式 BI 可视化分析平台 MVP

- **[2026-05-05] pnpm 严格依赖解析**: 从 npm 切换到 pnpm 后，antd 的传递依赖 `@ant-design/icons` 和 `dayjs` 必须显式安装。npm 会自动 hoist 这些依赖，pnpm 不会。这是 pnpm 的正确行为——依赖应该显式声明。

- **[2026-05-05] 分页查询必须有 ORDER BY**: 分页（LIMIT/OFFSET）如果没有 ORDER BY，返回的行子集是不确定的。之前代码在 `bun_builder.go`、`dialect.go`、`builder.go` 三处用 `ast.Pagination == nil` guard 跳过了 ORDER BY，导致分页排序失效。修复：移除 guard，让 ORDER BY 在有 sort 时始终生效。

- **[2026-05-05] 前端 prop mutation 反模式**: `TableChart.tsx` 直接修改 `queryConfig.sort` prop 而非通过回调。React 中不应 mutation props，应使用 `onSortChange` 回调让父组件更新 store。

- **[2026-05-05] adapter 兼容性模式**: 引入新类型（ChartSpec/QuerySpec）时，用 adapter 将旧请求转换为新类型，再用桥接函数转回旧 builder 参数。兼容性测试验证新旧路径产生相同 SQL。这样可以安全地逐步迁移，不破坏现有行为。

## Do-Not-Repeat

<!-- Mistakes made and corrected. Each entry prevents the same mistake recurring. -->
<!-- Format: [YYYY-MM-DD] Description of what went wrong and what to do instead. -->

## Decision Log

- **[2026-05-05] 分页无 sort 时不加默认 ORDER BY**: 当分页查询没有显式 sort 时，不自动添加默认排序。理由：调用方应负责排序语义，自动添加可能掩盖问题。数据库在无 ORDER BY 时返回的顺序是实现细节，不应依赖。

- [2026-05-06] ChartBuilder 定义驱动字段组索引规则：chartDefinitions 的 fieldGroups 是按整页顺序排列，但 queryConfig 的 dimensionGroups / metricGroups 是分开存储。渲染 QueryConfigRow、处理 drop zone 和组装请求时，不能直接复用 definition index，必须先映射到 kind 内部索引，否则指标会写入错误 group，最终请求 metrics 为空。

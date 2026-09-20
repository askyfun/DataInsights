# Data Insights 代码风格指南

## TypeScript 规范

### 配置文件

- `strict: true` - 严格模式
- `noUnusedLocals: true` - 未使用变量报错
- `noUnusedParameters: true` - 未使用参数报错
- 路径别名: `@/*` 指向 `./src/*`

### 前后端字段命名规范 (CRITICAL)

前后端 JSON 通信使用 **snake_case** 命名，确保字段完全一致。

```typescript
// ❌ 错误 - 字段名与后端不一致
interface DatasetColumn {
  dataType: string;    // 后端返回 type
  expression: string;  // 后端返回 expr
}

// ✅ 正确 - 与后端保持一致
interface DatasetColumn {
  type: string;
  expr: string;
  role: string;
  name: string;
  comment: string;
  typeConfig: TypeConfig;
}
```

常见对应关系：

| 后端 (Go JSON) | 前端 (TypeScript) | 用途 |
|----------------|-------------------|------|
| `data_type` | `dataType` | 数据源表结构 |
| `table_name` | `tableName` | 表名 |
| `query_sql` | `querySql` | 查询SQL |
| `query_config` | `queryConfig` | 图表查询配置 |
| `created_at` | `createdAt` | 创建时间 |

### 导入顺序

```typescript
// 1. React/库导入
import React, { useState, useEffect } from 'react';
import { Button, Table, Modal } from 'antd';
import { useNavigate } from 'react-router-dom';

// 2. 项目内部导入
import { datasourcesApi } from '@/api';
import { useStore } from '@/store';

// 3. 类型导入
import type { Datasource, Dataset } from '@/api';
```

### 命名规范

- 组件: PascalCase (`ChartBuilder.tsx`)
- 工具函数/变量: camelCase
- 常量: UPPER_SNAKE_CASE
- 禁止使用 `any`，使用 `unknown` 代替

---

## Go 规范

### 框架

使用 Gin (NOT go-zero)

### 包导入顺序

```go
import (
    "log/slog"
    "net/http"
    "os"

    "github.com/gin-gonic/gin"
    "github.com/uptrace/bun"

    "data-insights/internal/config"
    "data-insights/internal/database"
    "data-insights/internal/middleware"
    "data-insights/internal/model"
)
```

### 命名规范

- 函数/变量: camelCase
- 结构体/接口: PascalCase
- Error 变量: `ErrXXX` 或 `XXXError`

### Handler 模式

```go
func listDatasources(c *gin.Context) {
    db := c.MustGet("db").(*bun.DB)
    var datasources []model.Datasource
    if err := db.NewSelect().Model(&datasources).Scan(c.Request.Context()); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, datasources)
}
```

### 数据库模型 (bun ORM)

```go
type Datasource struct {
    bun.BaseModel `bun:"bi_datasource"`
    ID            int    `bun:"id,pk,autoincrement" json:"id"`
    Name          string `bun:"name" json:"name"`
    Host          string `bun:"host" json:"host"`
    // ...
}
```

### 错误处理

```go
// 正确
if err != nil {
    slog.Error("操作失败", "error", err)
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
}

// 避免
if err != nil {
    // 空块 - 禁止
}
```

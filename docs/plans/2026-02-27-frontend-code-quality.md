# Data Insights 前端代码质量提升实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 参照 Cherry Studio 的工程质量手段，全面提升 Data Insights 前端代码质量，包括： Biome 代码检查、Vitest 测试框架、pre-commit 强约束、CLAUDE.md AI 编码规范。

**Architecture:** 采用 "Biome 强约束 + TypeScript 硬检查 + Vitest 自动化测试 + AI 规范引导" 四位一体方案，与 Cherry Studio 保持一致。

**Tech Stack:** Biome, Vitest, TypeScript, lint-staged, husky

---

## 当前项目状态分析

### 已有的工程化手段
- ✅ TypeScript strict 模式
- ✅ 基础 tsconfig 配置 (paths 别名)
- ✅ Vite 构建工具

### 缺失的工程化手段
- ❌ 无代码检查工具 (ESLint/Prettier/Biome)
- ❌ 无单元测试框架
- ❌ 无 pre-commit 强约束
- ❌ 无 CLAUDE.md AI 编码规范
- ❌ 无 i18n 同步脚本

---

## Task 1: 安装 Biome 并配置

**Files:**
- Modify: `frontend/package.json`
- Create: `frontend/biome.json`

**Step 1: 安装 Biome**

```bash
cd frontend
npm install -D @biomejs/biome
```

**Step 2: 初始化 Biome 配置**

```bash
npx biome init
```

**Step 3: 配置 biome.json**

根据项目需求配置 (React + TypeScript + Ant Design):

```json
{
  "$schema": "https://biomejs.dev/schemas/1.9.0/schema.json",
  "organizeImports": {
    "enabled": true
  },
  "linter": {
    "enabled": true,
    "rules": {
      "recommended": true,
      "suspicious": {
        "noExplicitAny": "off"
      },
      "style": {
        "useImportType": "off"
      },
      "complexity": {
        "noForEach": "off"
      }
    }
  },
  "formatter": {
    "enabled": true,
    "indentStyle": "space",
    "indentWidth": 2,
    "lineWidth": 100
  },
  "javascript": {
    "formatter": {
      "quoteStyle": "single",
      "trailingCommas": "es5"
    }
  }
}
```

**Step 4: 更新 package.json scripts**

```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview",
    "format": "biome format --write .",
    "lint": "biome lint .",
    "check": "biome check ."
  }
}
```

**Step 5: 运行 Biome 检查并修复**

```bash
npm run check
```

预期输出: 显示当前代码风格问题

**Step 6: 提交**

```bash
git add frontend/package.json frontend/biome.json
git commit -m "chore(frontend): add Biome for lint and format"
```

---

## Task 2: 安装 Vitest 测试框架

**Files:**
- Modify: `frontend/package.json`
- Create: `frontend/vite.config.ts` (更新)
- Create: `frontend/src/__tests__/example.test.tsx`

**Step 1: 安装 Vitest 及相关依赖**

```bash
cd frontend
npm install -D vitest @vitejs/plugin-react jsdom @testing-library/react @testing-library/jest-dom
```

**Step 2: 更新 vite.config.ts 添加测试配置**

```typescript
/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 3000,
    open: false,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/__tests__/setup.ts',
    include: ['src/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
    },
  },
})
```

**Step 3: 创建测试配置文件**

```typescript
// frontend/src/__tests__/setup.ts
import '@testing-library/jest-dom'
```

**Step 4: 更新 package.json scripts**

```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview",
    "format": "biome format --write .",
    "lint": "biome lint .",
    "check": "biome check .",
    "test": "vitest",
    "test:ui": "vitest --ui",
    "test:coverage": "vitest --coverage"
  }
}
```

**Step 5: 创建示例测试**

```typescript
// frontend/src/__tests__/example.test.tsx
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'

describe('Example Test', () => {
  it('should pass', () => {
    expect(1 + 1).toBe(2)
  })
})
```

**Step 6: 运行测试验证**

```bash
npm test
```

预期输出: PASS

**Step 7: 提交**

```bash
git add frontend/package.json frontend/vite.config.ts frontend/src/__tests__
git commit -m "test(frontend): add Vitest testing framework"
```

---

## Task 3: 配置 pre-commit 强约束

**Files:**
- Modify: `frontend/package.json`
- Create: `frontend/.lintstagedrc.json`

**Step 1: 安装 husky 和 lint-staged**

```bash
cd frontend
npm install -D husky lint-staged
```

**Step 2: 初始化 husky**

```bash
npx husky init
```

**Step 3: 配置 .husky/pre-commit**

```bash
echo "npm run build:check" > .husky/pre-commit
```

**Step 4: 配置 lint-staged**

```json
// .lintstagedrc.json
{
  "*.{ts,tsx}": ["biome check --no-errors-on-unmatched"],
  "*.{json,css,md}": ["biome format --no-errors-on-unmatched"]
}
```

**Step 5: 更新 package.json 添加 build:check**

```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview",
    "format": "biome format --write .",
    "lint": "biome lint .",
    "check": "biome check .",
    "test": "vitest",
    "test:ui": "vitest --ui",
    "test:coverage": "vitest --coverage",
    "build:check": "npm run check && npm run test"
  },
  "lint-staged": {
    "*.{ts,tsx}": ["biome check --no-errors-on-unmatched"],
    "*.{json,css,md}": ["biome format --no-errors-on-unmatched"]
  }
}
```

**Step 6: 验证 pre-commit 钩子**

```bash
npm run build:check
```

预期输出: Biome check 通过 + 测试通过

**Step 7: 提交**

```bash
git add frontend/package.json frontend/.lintstagedrc.json frontend/.husky/
git commit -chore: add pre-commit hooks with build:check
```

---

## Task 4: 创建 CLAUDE.md AI 编码规范

**Files:**
- Create: `frontend/CLAUDE.md`

**Step 1: 创建 CLAUDE.md**

```markdown
# Data Insights Frontend Coding Standards

This document defines coding standards and conventions for AI assistants working on the Data Insights frontend codebase.

## Tech Stack

- React 18 + TypeScript
- Ant Design 5.x
- ECharts 5.x
- Zustand 4.x
- @dnd-kit (drag and drop)
- Vite 6
- Biome (linting + formatting)
- Vitest (testing)

## Code Style

### TypeScript

- **Always use strict TypeScript**: Enable `strict: true` in tsconfig
- **Never use `any`**: Use `unknown` instead when type is uncertain
- **Use explicit types**: Prefer explicit type annotations over type inference for function parameters and return values
- **Use snake_case for API**: All API communication uses snake_case (backend convention)

```typescript
// ✅ Correct
interface Datasource {
  id: number;
  name: string;
  host: string;
  port: number;
  database_name: string;
  created_at: string;
}

// ❌ Wrong
interface Datasource {
  id: any;
  name: string;
}
```

### React Components

- **Use functional components**: Never use class components
- **Use FC with explicit props**: Define prop types explicitly

```typescript
// ✅ Correct
interface ButtonProps {
  label: string;
  onClick: () => void;
}

export const Button: React.FC<ButtonProps> = ({ label, onClick }) => {
  return <button onClick={onClick}>{label}</button>
}

// ❌ Wrong
export const Button = ({ label, onClick }) => {
  return <button onClick={onClick}>{label}</button>
}
```

### Import Order

1. React/library imports
2. Project internal imports
3. Type imports

```typescript
// 1. React/Library
import React, { useState, useEffect } from 'react'
import { Button, Table, Modal } from 'antd'
import { useNavigate } from 'react-router-dom'

// 2. Project internal
import { datasourcesApi } from '@/api'
import { useStore } from '@/store'

// 3. Types
import type { Datasource, Dataset } from '@/api'
```

### Naming Conventions

- Components: PascalCase (`ChartBuilder.tsx`)
- Functions/variables: camelCase
- Constants: UPPER_SNAKE_CASE
- Files: kebab-case (except components)

### State Management (Zustand)

```typescript
// ✅ Correct - Use Zustand with TypeScript
interface StoreState {
  datasources: Datasource[]
  setDatasources: (datasources: Datasource[]) => void
}

export const useStore = create<StoreState>((set) => ({
  datasources: [],
  setDatasources: (datasources) => set({ datasources }),
}))
```

### Ant Design Usage

- Use Ant Design components consistently
- Follow Ant Design patterns for forms and tables
- Use `message` from antd for notifications

### API Calls

- Use the centralized API client in `@/lib/api/client`
- Always handle errors appropriately
- Use snake_case for API request/response

```typescript
// ✅ Correct
import { get, post } from '@/lib/api/client'

const result = await get<Datasource[]>('/api/datasources')
if (result.code === 20000) {
  const datasources = result.data
}
```

### Testing (Vitest)

- Write tests for all business logic
- Use @testing-library/react for component tests
- Follow AAA pattern: Arrange, Act, Assert

```typescript
describe('ComponentName', () => {
  it('should render correctly', () => {
    // Arrange
    const props = { /* ... */ }
    
    // Act
    render(<ComponentName {...props} />)
    
    // Assert
    expect(screen.getByText('Expected')).toBeInTheDocument()
  })
})
```

### Formatting

- Use Biome for formatting (2 spaces indent, 100 char line width)
- Run `npm run format` before committing
- Run `npm run check` to verify code quality

### Accessibility

- Use semantic HTML elements
- Add aria-label to icon buttons
- Ensure keyboard navigation works

## Project Structure

```
frontend/src/
├── api/              # API clients
├── components/       # Reusable components
├── idls/            # API type definitions
├── lib/             # Utilities
├── pages/           # Page components
├── store/           # Zustand stores
├── styles/          # Global styles
└── __tests__/       # Test files
```

## Commands

| Command | Description |
|---------|-------------|
| `npm run dev` | Start development server |
| `npm run build` | Build for production |
| `npm run format` | Format code with Biome |
| `npm run lint` | Lint with Biome |
| `npm run check` | Run Biome check |
| `npm run test` | Run tests |
| `npm run build:check` | Run full check (lint + test) |

## Pre-commit

Before committing, ensure:
1. `npm run build:check` passes
2. All tests pass
3. No TypeScript errors
</parameter>
```

**Step 2: 提交**

```bash
git add frontend/CLAUDE.md
git commit -docs: add CLAUDE.md for AI coding standards
```

---

## Task 5: 补充基础测试用例

**Files:**
- Create: `frontend/src/__tests__/api/client.test.ts`
- Create: `frontend/src/__tests__/store/index.test.ts`

**Step 1: 创建 API Client 测试**

```typescript
// frontend/src/__tests__/api/client.test.ts
import { describe, it, expect, vi } from 'vitest'

describe('API Client', () => {
  it('should export get, post, put, del functions', async () => {
    // Test will be implemented based on actual API client
    const api = await import('@/lib/api/client')
    expect(typeof api.get).toBe('function')
    expect(typeof api.post).toBe('function')
    expect(typeof api.put).toBe('function')
    expect(typeof api.del).toBe('function')
  })
})
```

**Step 2: 创建 Store 测试**

```typescript
// frontend/src/__tests__/store/index.test.ts
import { describe, it, expect } from 'vitest'
import { useStore } from '@/store'

describe('Store', () => {
  it('should have initial state', () => {
    const store = useStore.getState()
    expect(store.datasources).toEqual([])
    expect(store.datasets).toEqual([])
    expect(store.charts).toEqual([])
  })
})
```

**Step 3: 运行测试**

```bash
npm test
```

预期输出: 所有测试通过

**Step 4: 提交**

```bash
git add frontend/src/__tests__/
git commit -test: add basic test cases for API client and store
```

---

## Task 6: 更新文档

**Files:**
- Modify: `docs/setup.md`
- Modify: `docs/architecture.md`

**Step 1: 更新 docs/setup.md 添加前端工程化命令**

在 npm scripts 部分添加:

```markdown
## 前端工程化命令

| 命令 | 说明 |
|------|------|
| `npm run dev` | 开发模式 |
| `npm run build` | 构建生产版本 |
| `npm run preview` | 预览构建结果 |
| `npm run format` | 代码格式化 (Biome) |
| `npm run lint` | 代码检查 (Biome) |
| `npm run check` | 完整检查 |
| `npm run test` | 运行测试 (Vitest) |
| `npm run test:coverage` | 测试覆盖率 |
| `npm run build:check` | 完整检查 (lint + test + typecheck) |
```

**Step 2: 更新 docs/architecture.md 添加前端工程化部分**

添加:

```markdown
## 前端工程化

### 代码质量保证

| 工具 | 用途 |
|------|------|
| Biome | 代码格式化和 Lint |
| Vitest | 单元测试框架 |
| husky | Git pre-commit 钩子 |
| lint-staged | 提交前文件检查 |

### 质量检查流程

```
git commit
  ↓
pre-commit hook (husky)
  ↓
npm run build:check
  ├── biome check (lint)
  ├── vitest (test)
  └── tsc (typecheck)
  ↓
commit success / fail
```
```

**Step 3: 提交**

```bash
git add docs/setup.md docs/architecture.md
git commit -docs: update frontend engineering documentation
```

---

## 实施总结

### 完成后的工程化水平

| 手段 | Cherry Studio | Data Insights (实施前) | Data Insights (实施后) |
|------|---------------|-----------------|-----------------|
| Biome | ✅ | ❌ | ✅ |
| Vitest | ✅ | ❌ | ✅ |
| Pre-commit | ✅ | ❌ | ✅ |
| CLAUDE.md | ✅ | ❌ | ✅ |
| TypeScript 严格 | ✅ | ✅ | ✅ |

### 验证方式

1. 运行 `npm run build:check` 确保通过
2. 运行 `npm test` 确保测试通过
3. 尝试提交代码，确保 pre-commit 钩子触发
4. 查看 CLAUDE.md 确认 AI 编码规范存在

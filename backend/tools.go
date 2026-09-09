//go:build tools

// tools 依赖登记文件：仅用于将 API 代码生成工具纳入 go.mod 管理（go mod tidy
// 依赖本文件保留工具依赖），被 go:build 排除在常规构建之外。
// 工具执行入口：make api-gen（仓库根）。
package tools

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
)

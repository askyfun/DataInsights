package expr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSharedCases 读取**前后端共用**的表达式用例表
// （`frontend/src/lib/__fixtures__/exprCases.json`），逐例校验 Go 侧结果。
//
// 这张表是编辑器 TS 镜像（frontend/src/lib/expr.ts）与后端围栏之间的防漂移机制：
// 任何一侧单独改语法，都会让这里或前端 `expr.cases.test.ts` 变红。
func TestSharedCases(t *testing.T) {
	const fixturePath = "../../../frontend/src/lib/__fixtures__/exprCases.json"
	raw, err := os.ReadFile(filepath.Clean(fixturePath))
	if err != nil {
		t.Fatalf("读取共用用例表失败（双端一致性测试依赖它）: %v", err)
	}

	var fixture struct {
		Cases []struct {
			Name  string `json:"name"`
			Expr  string `json:"expr"`
			Valid bool   `json:"valid"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("解析共用用例表失败: %v", err)
	}
	if len(fixture.Cases) < 40 {
		t.Fatalf("用例表疑似被截断：只有 %d 例", len(fixture.Cases))
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			err := Validate(tc.Expr)
			if tc.Valid && err != nil {
				t.Fatalf("Validate(%q) = %v, 表格要求放行", tc.Expr, err)
			}
			if !tc.Valid && err == nil {
				t.Fatalf("Validate(%q) = nil, 表格要求拒绝", tc.Expr)
			}
		})
	}
}

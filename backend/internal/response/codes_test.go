package response

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// R-99（issue #224）四方一致性守卫：response 常量表 ↔ 双端共读 fixture ↔
// docs/developer-guide/api.md §1.3 稳定码表 ↔ api/openapi.yaml 的
// ErrorDetail.code 枚举。任一增删改不同步即测试红。沿用
// dateFilterCases.json 的双端共读做法（fixture 归前端目录，后端跨目录读）。

const errCodesFixturePath = "../../../frontend/src/lib/__fixtures__/errorCodes.json"
const apiMdPath = "../../../docs/developer-guide/api.md"
const openapiPath = "../../../api/openapi.yaml"

type errCodeCase struct {
	BizCode int    `json:"bizCode"`
	Stable  string `json:"stable"`
	HintKey string `json:"hintKey"`
}

type errCodesFixture struct {
	Comment string        `json:"comment"`
	Codes   []errCodeCase `json:"codes"`
}

func loadErrCodesFixture(t *testing.T) errCodesFixture {
	t.Helper()
	raw, err := os.ReadFile(errCodesFixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f errCodesFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(f.Codes) == 0 {
		t.Fatal("fixture has no codes")
	}
	return f
}

// stableHints 是 Go 侧唯一事实表：常量直接参与编译，fixture 漂移即红。
var stableHints = map[string]string{
	StableUnauthenticated:       HintUnauthenticated,
	StableForbidden:             HintForbidden,
	StableDatasourceUnreachable: HintDatasourceUnreachable,
	StableQueryExecFailed:       HintQueryExecFailed,
	StableInvalidConfig:         HintInvalidConfig,
}

// TestStableCodesMatchFixture 校验常量表与 fixture 逐条相等（双向）。
func TestStableCodesMatchFixture(t *testing.T) {
	f := loadErrCodesFixture(t)
	if len(f.Codes) != len(stableHints) {
		t.Fatalf("fixture has %d codes, Go table has %d", len(f.Codes), len(stableHints))
	}
	for _, c := range f.Codes {
		hint, ok := stableHints[c.Stable]
		if !ok {
			t.Errorf("fixture stable %q not in Go constants", c.Stable)
			continue
		}
		if hint != c.HintKey {
			t.Errorf("stable %q: Go hint %q != fixture %q", c.Stable, hint, c.HintKey)
		}
	}
	// bizCode 映射：20200/20201 在 stableByBizCode；三个 50000 档不在。
	for _, c := range f.Codes {
		_, mapped := stableByBizCode[c.BizCode]
		if c.BizCode == CodeUnauthorized || c.BizCode == CodeForbidden {
			if !mapped {
				t.Errorf("biz %d (%s) must map in stableByBizCode", c.BizCode, c.Stable)
			}
		} else if mapped {
			t.Errorf("biz %d must NOT map in stableByBizCode (uncoded 50000 family)", c.BizCode)
		}
	}
	if CodeForbidden != 20201 {
		t.Errorf("CodeForbidden = %d, want 20201", CodeForbidden)
	}
}

// TestStableCodesMatchOpenapi 解析 openapi.yaml 的 ErrorDetail.code 枚举与
// ResponseCode enum（须含 20201）。用正则抽 enum 块，避免引入 yaml 依赖。
func TestStableCodesMatchOpenapi(t *testing.T) {
	raw, err := os.ReadFile(openapiPath)
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	spec := string(raw)

	f := loadErrCodesFixture(t)
	want := map[string]bool{}
	for _, c := range f.Codes {
		want[c.Stable] = true
	}

	enumRe := regexp.MustCompile(`(?s)ErrorDetail:[\s\S]*?code:\s*\n(.*?)\n\s+hint_key`)
	m := enumRe.FindStringSubmatch(spec)
	if m == nil {
		t.Fatal("ErrorDetail.code enum block not found in openapi.yaml")
	}
	// 只认 "- STABLE_CODE" 形式的行；x-enum-varnames 的 Go 命名不在捕获内。
	itemRe := regexp.MustCompile(`^\s*-\s+([A-Z][A-Z_]+)\s*$`)
	got := map[string]bool{}
	for _, line := range strings.Split(m[1], "\n") {
		if im := itemRe.FindStringSubmatch(line); im != nil {
			got[im[1]] = true
		}
	}
	for s := range want {
		if !got[s] {
			t.Errorf("stable %q missing from openapi ErrorDetail.code enum", s)
		}
	}
	for s := range got {
		if !want[s] {
			t.Errorf("stable %q in openapi enum but not in fixture", s)
		}
	}

	if !strings.Contains(spec, "        - 20201") {
		t.Error("ResponseCode enum must include 20201 (CodeForbidden)")
	}
}

// TestStableCodesMatchApiMd 解析 api.md §1.3 的稳定码表（| error.code | … |
// hint_key |）与 fixture 对表。
func TestStableCodesMatchApiMd(t *testing.T) {
	raw, err := os.ReadFile(apiMdPath)
	if err != nil {
		t.Fatalf("read api.md: %v", err)
	}
	f := loadErrCodesFixture(t)
	want := map[string]string{} // stable -> hintKey
	for _, c := range f.Codes {
		want[c.Stable] = c.HintKey
	}

	got := map[string]string{}
	inTable := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, "### 1.3") {
			inTable = true
			continue
		}
		if !inTable || !strings.HasPrefix(line, "|") {
			continue
		}
		cols := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cols {
			cols[i] = strings.TrimSpace(cols[i])
		}
		// 稳定码表行：首列是反引号包裹的 STABLE_CODE，hint_key 列以 errors. 开头。
		if len(cols) >= 3 && strings.HasPrefix(cols[0], "`") && strings.HasSuffix(cols[0], "`") &&
			strings.Contains(line, "errors.") {
			stable := strings.Trim(cols[0], "`")
			if stable == "error.code" || stable == "---" {
				continue
			}
			hint := ""
			for _, col := range cols[1:] {
				if strings.HasPrefix(col, "`errors.") {
					hint = strings.Trim(col, "`")
					break
				}
			}
			got[stable] = hint
		}
	}
	for s, h := range want {
		if got[s] != h {
			t.Errorf("api.md stable table: %q hint = %q, want %q", s, got[s], h)
		}
	}
	for s := range got {
		if _, ok := want[s]; !ok {
			t.Errorf("api.md stable table has extra code %q", s)
		}
	}
}

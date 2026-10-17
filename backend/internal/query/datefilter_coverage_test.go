package query

// 日期筛选的**分支覆盖**门禁：把「代码支持的 preset」与「双端共用用例表覆盖到的 preset」
// 求差集，差集非空即失败。
//
// 为什么要有这个测试：`dateFilterCases.json` 是防漂移的唯一机制，但它只保证
// 「表里有的用例」双端一致，不保证「代码里支持的分支」都在表里。2026-10-17 一轮
// 变异测试就发现 `last52w` 双端都实现了、用例表却一例都没有——这类缺口靠人肉Review
// 抓不住，且它的代价是静默的（镜像逻辑少一条断言，改错了也没人红）。
//
// 契约源选 `DynamicPresetKey`（TS union）而不是扫 Go 源码里的 `case "…"`：
// 根 AGENTS.md 写明 `lib/dateFilter.ts` 是日期语义的单一事实源；且文本扫 switch 分支
// 本身会漂移（换行/常量拼接就漏）。这里用**行为探针**判定 Go 侧是否真的支持某个
// preset——把 preset 喂进导出的 ResolveDateFilter，看它是否给出区间。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	coverageTSPath   = "../../../frontend/src/lib/dateFilter.ts"
	coverageFixture  = "../../../frontend/src/lib/__fixtures__/dateFilterCases.json"
	coverageUnionTyp = "export type DynamicPresetKey ="
)

// presetQuoted 取 union 体里的字符串字面量（注释里没有引号，不会误伤）。
var presetQuoted = regexp.MustCompile(`'([A-Za-z0-9]+)'`)

// TestDateFilterPresetBranchCoverage 三条断言，方向互补：
//  1. union 声明的每个 preset，Go 镜像必须**可达**（switch 不能漏分支）；
//  2. union 声明的每个 preset，用例表必须有**至少一例**（堵住 last52w 式缺口）；
//  3. 用例表出现的每个 preset，必须仍在 union 里声明（表里不许留幽灵）。
func TestDateFilterPresetBranchCoverage(t *testing.T) {
	union := readUnionPresets(t)
	table := readTablePresets(t)

	// 断言 1 + 2：以 TS union 为准逐个探。
	for _, p := range union {
		t.Run(p, func(t *testing.T) {
			if !goPresetReachable(t, p) {
				t.Errorf("preset %q 在 TS union 中声明，但 Go 侧 resolvePresetRange 不可达（镜像漏分支）", p)
			}
			if table[p] == 0 {
				t.Errorf("preset %q 双端都可能支持，但共用用例表零覆盖——加一例进 dateFilterCases.json", p)
			}
		})
	}

	// 断言 3：表里不许有 union 没声明的 preset。
	inUnion := make(map[string]bool, len(union))
	for _, p := range union {
		inUnion[p] = true
	}
	for p, n := range table {
		if !inUnion[p] {
			t.Errorf("用例表出现 union 未声明的 preset %q（%d 例）：幽灵用例会让双端各自兜底掩盖漂移", p, n)
		}
	}
}

// goPresetReachable 用导出口径探测：dynamic + 该 preset 必须解析出 between 区间。
func goPresetReachable(t *testing.T, preset string) bool {
	t.Helper()
	now := coverageNow(t)
	got := ResolveDateFilter(DateFilterValue{Kind: "dynamic", Preset: preset}, "day", 1, false, now)
	return got.Operator == "between" && got.Start != "" && got.End != ""
}

// coverageNow 取用例表的基准时刻，保证探测与用例跑的是同一个「今天」。
func coverageNow(t *testing.T) time.Time {
	t.Helper()
	var meta struct {
		Now string `json:"now"`
	}
	raw := readFile(t, coverageFixture)
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("解析用例表失败: %v", err)
	}
	now, err := time.ParseInLocation("2006-01-02T15:04:05", meta.Now, time.Local)
	if err != nil {
		t.Fatalf("用例表 now %q 解析失败: %v", meta.Now, err)
	}
	return now
}

// readUnionPresets 抓出 union 声明的全部 preset；一条都没抓到就失败（防正则失效后静默通过）。
func readUnionPresets(t *testing.T) []string {
	t.Helper()
	text := string(readFile(t, coverageTSPath))
	at := strings.Index(text, coverageUnionTyp)
	if at < 0 {
		t.Fatalf("没找到 %q：TS union 写法变了，同步修改本测试", coverageUnionTyp)
	}
	body := text[at:]
	end := strings.Index(body, ";")
	if end < 0 {
		t.Fatal("没找到 union 声明的结束分号：TS union 写法变了，同步修改本测试")
	}
	body = body[:end]

	var out []string
	seen := make(map[string]bool)
	for _, m := range presetQuoted.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	if len(out) == 0 {
		t.Fatalf("从 %s 解析到 0 个 preset：union 写法变了，本测试的判据已失效，必须同步修改它", coverageUnionTyp)
	}
	return out
}

// readTablePresets 统计用例表里每个 dynamic preset 出现了几例。
func readTablePresets(t *testing.T) map[string]int {
	t.Helper()
	var fixture struct {
		Cases []struct {
			Value DateFilterValue `json:"value"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(readFile(t, coverageFixture), &fixture); err != nil {
		t.Fatalf("解析用例表失败: %v", err)
	}
	counts := make(map[string]int)
	for _, c := range fixture.Cases {
		if c.Value.Kind == "dynamic" && c.Value.Preset != "" {
			counts[c.Value.Preset]++
		}
	}
	if len(counts) == 0 {
		t.Fatal("用例表里一条 dynamic preset 都没有，覆盖检查失去意义")
	}
	return counts
}

func readFile(t *testing.T, rel string) []byte {
	t.Helper()
	// go test 的工作目录是本包目录，故相对路径指向仓库根下的前端文件。
	raw, err := os.ReadFile(filepath.Clean(rel))
	if err != nil {
		t.Fatalf("读取 %s 失败（覆盖门禁依赖它）: %v", rel, err)
	}
	return raw
}

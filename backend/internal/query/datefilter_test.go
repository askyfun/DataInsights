package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDateFilterSharedCases 读取**前后端共用**的用例表
// （`frontend/src/lib/__fixtures__/dateFilterCases.json`），逐例校验 Go 侧解析结果。
//
// 这张表是防漂移的唯一机制：后端镜像了一份 TS 的日期语义（见 datefilter.go 顶部说明），
// 任何一侧单独改动都会让这里或前端 `dateFilter.cases.test.ts` 变红。
// 前端那一侧由已实现且独立验证过的 TS 解析器跑同一张表，因此「表本身写错」也会被抓住。
func TestDateFilterSharedCases(t *testing.T) {
	// go test 的工作目录是包目录，故相对路径指向仓库根下的前端夹具。
	const fixturePath = "../../../frontend/src/lib/__fixtures__/dateFilterCases.json"
	raw, err := os.ReadFile(filepath.Clean(fixturePath))
	if err != nil {
		t.Fatalf("读取共用用例表失败（双端一致性测试依赖它）: %v", err)
	}

	var fixture struct {
		Now   string `json:"now"`
		Cases []struct {
			Name        string          `json:"name"`
			Now         string          `json:"now"`
			Granularity string          `json:"granularity"`
			WeekStart   int             `json:"weekStart"`
			WithTime    bool            `json:"withTime"`
			Value       DateFilterValue `json:"value"`
			Want        struct {
				// 指针区分「没给 operator」与「显式 null」：两者都表示不下发条件。
				Operator     *string `json:"operator"`
				Start        string  `json:"start"`
				End          string  `json:"end"`
				IncludeEmpty bool    `json:"includeEmpty"`
			} `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("解析共用用例表失败: %v", err)
	}
	if len(fixture.Cases) < 30 {
		t.Fatalf("用例表疑似被截断：只有 %d 例", len(fixture.Cases))
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			nowText := tc.Now
			if nowText == "" {
				nowText = fixture.Now
			}
			// 两侧都按本地时区解释 now（前端 dayjs 也是本地解析）。用 ParseInLocation 是为了
			// 避免 time.Parse 默认掉进 UTC，导致"同一天"在不同时区下差一天。
			now, err := time.ParseInLocation("2006-01-02T15:04:05", nowText, time.Local)
			if err != nil {
				t.Fatalf("now %q 解析失败: %v", nowText, err)
			}

			got := ResolveDateFilter(tc.Value, tc.Granularity, tc.WeekStart, tc.WithTime, now)

			wantOperator := ""
			if tc.Want.Operator != nil {
				wantOperator = *tc.Want.Operator
			}
			if got.Operator != wantOperator {
				t.Errorf("operator = %q, want %q", got.Operator, wantOperator)
			}
			if got.Start != tc.Want.Start {
				t.Errorf("start = %q, want %q", got.Start, tc.Want.Start)
			}
			if got.End != tc.Want.End {
				t.Errorf("end = %q, want %q", got.End, tc.Want.End)
			}
			if got.IncludeEmpty != tc.Want.IncludeEmpty {
				t.Errorf("includeEmpty = %v, want %v", got.IncludeEmpty, tc.Want.IncludeEmpty)
			}
		})
	}
}

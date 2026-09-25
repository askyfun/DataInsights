package query

import (
	"encoding/json"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// 日期筛选意图的 Go 侧解析（前端 frontend/src/lib/dateFilter.ts 的镜像）
//
// 为什么后端要镜像一份：图表的日期条件随 config 落库，而**分享页与仪表盘的取数是后端自己
// 读 config 组查询**的 —— 前端没参与，所以「最近 7 天」这类意图只能由后端解析；否则它永远
// 停在保存那一刻写下的区间快照上（仪表盘是个长期挂着的监控面，这个偏差是实打实的错数）。
//
// ⚠️ 语义必须与 TS 逐例一致：两侧共读
// frontend/src/lib/__fixtures__/dateFilterCases.json（本包 datefilter_test.go 会读它）。
// 改任何一侧的语义都必须同时改另一侧并跑两边的用例，否则该表会直接抓出漂移。
//
// 时区口径：一律按**服务器本地时区**的「日」计算。前端按浏览器本地日；同一套部署下两者一致，
// 跨时区多地域部署会有一天边界差 —— 与「数据集本身按业务时区落数」的既有假设配套，属已知取舍。
// ---------------------------------------------------------------------------

const (
	DateGranularityHour  = "hour"
	DateGranularityDay   = "day"
	DateGranularityWeek  = "week"
	DateGranularityMonth = "month"

	dateLayout     = "2006-01-02"
	dateTimeLayout = "2006-01-02 15:04:05"

	// dateDefaultWeekStart 周计算逻辑缺省值：周一（与前端一致）。
	dateDefaultWeekStart = 1
)

// DateFilterIntent 是前端 FilterCondition.date 的镜像。
type DateFilterIntent struct {
	Value       DateFilterValue `json:"value"`
	Granularity string          `json:"granularity"`
	// WeekStart 缺省 = 周一；用指针区分「没给」与「显式 0（周日）」。
	WeekStart *int `json:"weekStart"`
	WithTime  bool `json:"withTime"`
}

// DateFilterValue 五种模式共用一套扁平字段：`start`/`end` 在 fixed 下是日期串、在 advanced 下
// 是端点对象，故用 RawMessage 按 kind 二次解析，而不是把它们拆成两套结构体。
type DateFilterValue struct {
	Kind         string           `json:"kind"`
	Preset       string           `json:"preset"`
	Custom       *DateCustomValue `json:"custom"`
	IncludeEmpty bool             `json:"includeEmpty"`
	Start        json.RawMessage  `json:"start"`
	End          json.RawMessage  `json:"end"`
	// Value 仅 special 使用：empty / notEmpty / all。
	Value string `json:"value"`
	// Date 仅 single 使用。
	Date string `json:"date"`
}

// DateCustomValue 自定义动态日期（最近 / 前 / 后 N 个单位）。
type DateCustomValue struct {
	Op           string `json:"op"`
	N            int    `json:"n"`
	Unit         string `json:"unit"`
	IncludeToday bool   `json:"includeToday"`
	// OnTheHour 缺省 = 对齐整点（与前端一致）；显式 false 才退化成相对当前时刻的滚动窗口。
	OnTheHour          *bool `json:"onTheHour"`
	IncludeCurrentHour bool  `json:"includeCurrentHour"`
}

// DateBound 是高级模式的单侧端点（fixed / dynamic / unlimited）。
type DateBound struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Op    string `json:"op"`
	N     int    `json:"n"`
	Unit  string `json:"unit"`
}

// ResolvedDateFilter 是解析结果。Operator 为空串表示「这条不下发任何条件」
// （未选择、「所有日期」、起止同时无限制、畸形输入）。
type ResolvedDateFilter struct {
	Operator     string
	Start        string
	End          string
	IncludeEmpty bool
}

// ---------------------------------------------------------------------------
// 日期运算（全部锚定到「天/小时/月首」边界，避免 AddDate 的月末溢出）
// ---------------------------------------------------------------------------

func dateAtDay(t time.Time, hour, min, sec int) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), hour, min, sec, 0, t.Location())
}

func startOfDay(t time.Time) time.Time  { return dateAtDay(t, 0, 0, 0) }
func endOfDay(t time.Time) time.Time    { return dateAtDay(t, 23, 59, 59) }
func startOfHour(t time.Time) time.Time { return dateAtDay(t, t.Hour(), 0, 0) }
func endOfHour(t time.Time) time.Time   { return dateAtDay(t, t.Hour(), 59, 59) }

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// endOfMonth 从「本月 1 日」推进到下月 1 日再退 1 秒：先归到 1 日，避开 AddDate 的月末溢出。
func endOfMonth(t time.Time) time.Time {
	return startOfMonth(t).AddDate(0, 1, 0).Add(-time.Second)
}

func startOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, t.Location())
}

func endOfYear(t time.Time) time.Time {
	return time.Date(t.Year()+1, time.January, 1, 0, 0, 0, 0, t.Location()).Add(-time.Second)
}

// startOfWeek 按「周计算逻辑」求当周首日（星期几的 0=周日 与前端一致）。
func startOfWeek(t time.Time, weekStart int) time.Time {
	shift := (int(t.Weekday()) - weekStart + 7) % 7
	return startOfDay(t.AddDate(0, 0, -shift))
}

func endOfWeek(t time.Time, weekStart int) time.Time {
	return endOfDay(startOfWeek(t, weekStart).AddDate(0, 0, 6))
}

// startOfBiMonth 双月分组以 1 月为起点两两成组（1~2 月、3~4 月 …），与前端一致。
func startOfBiMonth(t time.Time) time.Time {
	month := time.Month((int(t.Month())-1)/2*2 + 1)
	return time.Date(t.Year(), month, 1, 0, 0, 0, 0, t.Location())
}

// endOfBiMonth 从组首推进两个月再退 1 秒（同 endOfMonth 的防溢出写法）。
func endOfBiMonth(t time.Time) time.Time {
	return startOfBiMonth(t).AddDate(0, 2, 0).Add(-time.Second)
}

func startOfQuarter(t time.Time) time.Time {
	month := time.Month((int(t.Month())-1)/3*3 + 1)
	return time.Date(t.Year(), month, 1, 0, 0, 0, 0, t.Location())
}

func endOfQuarter(t time.Time) time.Time {
	return startOfQuarter(t).AddDate(0, 3, 0).Add(-time.Second)
}

func startOfUnit(t time.Time, unit string, weekStart int) time.Time {
	switch unit {
	case "hour":
		return startOfHour(t)
	case "week":
		return startOfWeek(t, weekStart)
	case "month":
		return startOfMonth(t)
	case "year":
		return startOfYear(t)
	default:
		return startOfDay(t)
	}
}

func endOfUnit(t time.Time, unit string, weekStart int) time.Time {
	switch unit {
	case "hour":
		return endOfHour(t)
	case "week":
		return endOfWeek(t, weekStart)
	case "month":
		return endOfMonth(t)
	case "year":
		return endOfYear(t)
	default:
		return endOfDay(t)
	}
}

// parseDateValue 解析 `YYYY-MM-DD[ HH:mm[:ss]]`（也接受 `T` 分隔）。无法解析返回零值 + false。
// 按服务器本地时区解释：这里的日期代表「数据里的那一天的本地零点」，不是 UTC 时刻。
func parseDateValue(input string) (time.Time, bool) {
	normalized := strings.TrimSpace(strings.ReplaceAll(input, "T", " "))
	if normalized == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{dateTimeLayout, "2006-01-02 15:04", dateLayout} {
		if parsed, err := time.ParseInLocation(layout, normalized, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func formatMoment(t time.Time, usesTime bool) string {
	if usesTime {
		return t.Format(dateTimeLayout)
	}
	return t.Format(dateLayout)
}

// ---------------------------------------------------------------------------
// 解析
// ---------------------------------------------------------------------------

func normalizeWeekStart(input *int) int {
	if input == nil || *input < 0 || *input > 6 {
		return dateDefaultWeekStart
	}
	return *input
}

// DateWeekStartOr 取意图里的周计算逻辑，缺省周一（与前端 DateFilterSettings 的缺省一致）。
func DateWeekStartOr(input *int) int { return normalizeWeekStart(input) }

// emptyResolution 表示「这条不下发任何条件」。
func emptyResolution() ResolvedDateFilter { return ResolvedDateFilter{} }

// resolvePresetRange 快捷选项 → 区间。全部以「昨天」为数据上界（文档口径）。
func resolvePresetRange(preset string, now time.Time, weekStart int) (time.Time, time.Time, bool) {
	yesterday := now.AddDate(0, 0, -1)
	yesterdayEnd := endOfDay(yesterday)
	monthsAgoStart := func(months int) time.Time {
		return startOfMonth(now).AddDate(0, -months, 0)
	}
	// 周/月/季度/双月/年一律先归到「当日」「当 1 日」再位移，避免 AddDate 的月末溢出把月份算歪。
	previous := func(months int) time.Time { return startOfMonth(now).AddDate(0, -months, 0) }

	switch preset {
	case "last1d":
		return startOfDay(yesterday), yesterdayEnd, true
	case "last7d":
		return startOfDay(now.AddDate(0, 0, -7)), yesterdayEnd, true
	case "last14d":
		return startOfDay(now.AddDate(0, 0, -14)), yesterdayEnd, true
	case "last30d":
		return startOfDay(now.AddDate(0, 0, -30)), yesterdayEnd, true
	case "last365d":
		return startOfDay(now.AddDate(0, 0, -365)), yesterdayEnd, true

	case "last1w":
		return startOfWeek(now, weekStart), yesterdayEnd, true
	case "last4w":
		return startOfWeek(now, weekStart).AddDate(0, 0, -21), yesterdayEnd, true
	case "last13w":
		return startOfWeek(now, weekStart).AddDate(0, 0, -84), yesterdayEnd, true
	case "last52w":
		return startOfWeek(now, weekStart).AddDate(0, 0, -357), yesterdayEnd, true

	case "last1m":
		return monthsAgoStart(0), yesterdayEnd, true
	case "last3m":
		return monthsAgoStart(2), yesterdayEnd, true
	case "last6m":
		return monthsAgoStart(5), yesterdayEnd, true
	case "last12m":
		return monthsAgoStart(11), yesterdayEnd, true

	case "thisWeek":
		return startOfWeek(now, weekStart), yesterdayEnd, true
	case "lastWeek":
		lastWeekDay := now.AddDate(0, 0, -7)
		return startOfWeek(lastWeekDay, weekStart), endOfWeek(lastWeekDay, weekStart), true

	case "thisMonth":
		return startOfMonth(now), yesterdayEnd, true
	case "lastMonth":
		return previous(1), endOfMonth(previous(1)), true

	case "thisBiMonth":
		return startOfBiMonth(now), yesterdayEnd, true
	case "lastBiMonth":
		return startOfBiMonth(previous(2)), endOfBiMonth(previous(2)), true

	case "thisQuarter":
		return startOfQuarter(now), yesterdayEnd, true
	case "lastQuarter":
		return startOfQuarter(previous(3)), endOfQuarter(previous(3)), true

	case "thisYear":
		return startOfYear(now), yesterdayEnd, true
	case "last2y":
		return startOfYear(time.Date(now.Year()-1, time.January, 1, 0, 0, 0, 0, now.Location())), yesterdayEnd, true
	case "lastYear":
		lastYear := time.Date(now.Year()-1, time.January, 1, 0, 0, 0, 0, now.Location())
		return startOfYear(lastYear), endOfYear(lastYear), true

	default:
		return time.Time{}, time.Time{}, false
	}
}

// resolveCustomRange 自定义动态日期。返回优先级：range（双端）> upperOnly > lowerOnly。
func resolveCustomRange(
	custom DateCustomValue, now time.Time, weekStart int,
) (start, end time.Time, upperOnly, lowerOnly time.Time, ok bool) {
	n := custom.N
	if n < 0 {
		n = 0
	}

	if custom.Unit == "hour" {
		aligned := custom.OnTheHour == nil || *custom.OnTheHour
		if custom.Op != "last" {
			anchor := now
			if aligned {
				anchor = startOfHour(now)
			}
			if custom.Op == "before" {
				shifted := anchor.Add(-time.Duration(n) * time.Hour)
				if aligned {
					return time.Time{}, time.Time{}, endOfHour(shifted), time.Time{}, true
				}
				return time.Time{}, time.Time{}, shifted, time.Time{}, true
			}
			return time.Time{}, time.Time{}, time.Time{}, anchor.Add(time.Duration(n) * time.Hour), true
		}
		if !aligned {
			return now.Add(-time.Duration(n) * time.Hour), now, time.Time{}, time.Time{}, true
		}
		start = startOfHour(now.Add(-time.Duration(n) * time.Hour))
		if custom.IncludeCurrentHour {
			end = endOfHour(now)
		} else {
			end = endOfHour(now.Add(-time.Hour))
		}
		return start, end, time.Time{}, time.Time{}, true
	}

	if custom.Op == "last" {
		// 周/月/年「最近 N 个」= 含当期在内的 N 个周期首日 ~ 昨天（与前端一致）。
		switch custom.Unit {
		case "week", "month":
			start = startOfUnit(now, custom.Unit, weekStart)
			if custom.Unit == "week" {
				start = start.AddDate(0, 0, -7*(n-1))
			} else {
				start = start.AddDate(0, -(n - 1), 0)
			}
		case "year":
			start = time.Date(now.Year()-(n-1), time.January, 1, 0, 0, 0, 0, now.Location())
		default:
			start = startOfDay(now.AddDate(0, 0, -n))
		}
		if custom.IncludeToday {
			end = endOfDay(now)
		} else {
			end = endOfDay(now.AddDate(0, 0, -1))
		}
		return start, end, time.Time{}, time.Time{}, true
	}

	anchor := startOfUnit(now, custom.Unit, weekStart)
	if custom.Op == "before" {
		shifted := shiftUnit(anchor, custom.Unit, -n)
		return time.Time{}, time.Time{}, endOfUnit(shifted, custom.Unit, weekStart), time.Time{}, true
	}
	return time.Time{}, time.Time{}, time.Time{}, startOfUnit(shiftUnit(anchor, custom.Unit, n), custom.Unit, weekStart), true
}

// shiftUnit 按单位位移 n 个周期。月/年走「先归 1 日/1 月」再位移，避免月末溢出。
func shiftUnit(t time.Time, unit string, n int) time.Time {
	switch unit {
	case "hour":
		return t.Add(time.Duration(n) * time.Hour)
	case "week":
		return t.AddDate(0, 0, 7*n)
	case "month":
		return t.AddDate(0, n, 0)
	case "year":
		return t.AddDate(n, 0, 0)
	default:
		return t.AddDate(0, 0, n)
	}
}

// resolveBound 高级模式单侧端点 → 时刻；unlimited 返回零值 + false 表示该侧不设界。
func resolveBound(
	bound DateBound, role string, now time.Time, weekStart int,
) (time.Time, bool) {
	switch bound.Type {
	case "unlimited":
		return time.Time{}, false
	case "fixed":
		parsed, ok := parseDateValue(bound.Value)
		return parsed, ok
	case "dynamic":
		n := bound.N
		if n < 0 {
			n = 0
		}
		anchor := startOfUnit(now, bound.Unit, weekStart)
		offset := n
		if bound.Op == "before" {
			offset = -n
		}
		shifted := shiftUnit(anchor, bound.Unit, offset)
		if role == "start" {
			return startOfUnit(shifted, bound.Unit, weekStart), true
		}
		return endOfUnit(shifted, bound.Unit, weekStart), true
	default:
		return time.Time{}, false
	}
}

// ResolveDateFilter 把日期筛选意图解析成下发的过滤语义。
//
// 返回 Operator 为空串的四种情况（都表示「不产生过滤条件」）：未选择、特殊值「所有日期」、
// 起止同时「无限制」、畸形输入。畸形输入一律收敛到「不过滤」而不是凭空造区间。
func ResolveDateFilter(
	value DateFilterValue, granularity string, weekStart int, withTime bool, now time.Time,
) ResolvedDateFilter {
	usesTime := granularity == DateGranularityHour || withTime
	ws := weekStart
	if ws < 0 || ws > 6 {
		ws = dateDefaultWeekStart
	}
	format := func(t time.Time) string { return formatMoment(t, usesTime) }

	switch value.Kind {
	case "dynamic":
		if value.Custom != nil {
			start, end, upperOnly, lowerOnly, ok := resolveCustomRange(*value.Custom, now, ws)
			if !ok {
				return emptyResolution()
			}
			switch {
			case !start.IsZero() && !end.IsZero():
				return ResolvedDateFilter{
					Operator:     "between",
					Start:        format(start),
					End:          format(end),
					IncludeEmpty: value.IncludeEmpty,
				}
			case !upperOnly.IsZero():
				return ResolvedDateFilter{Operator: "lte", End: format(upperOnly), IncludeEmpty: value.IncludeEmpty}
			case !lowerOnly.IsZero():
				return ResolvedDateFilter{Operator: "gte", Start: format(lowerOnly), IncludeEmpty: value.IncludeEmpty}
			default:
				return emptyResolution()
			}
		}
		if value.Preset == "" {
			return emptyResolution()
		}
		start, end, ok := resolvePresetRange(value.Preset, now, ws)
		if !ok {
			return emptyResolution()
		}
		return ResolvedDateFilter{
			Operator:     "between",
			Start:        format(start),
			End:          format(end),
			IncludeEmpty: value.IncludeEmpty,
		}

	case "fixed":
		var startText, endText string
		_ = json.Unmarshal(value.Start, &startText)
		_ = json.Unmarshal(value.End, &endText)
		start, okStart := parseDateValue(startText)
		end, okEnd := parseDateValue(endText)
		if !okStart || !okEnd {
			return emptyResolution()
		}
		return ResolvedDateFilter{
			Operator:     "between",
			Start:        format(start),
			End:          format(end),
			IncludeEmpty: value.IncludeEmpty,
		}

	case "advanced":
		var rawStart, rawEnd DateBound
		_ = json.Unmarshal(value.Start, &rawStart)
		_ = json.Unmarshal(value.End, &rawEnd)
		start, hasStart := resolveBound(rawStart, "start", now, ws)
		end, hasEnd := resolveBound(rawEnd, "end", now, ws)
		switch {
		case hasStart && hasEnd:
			return ResolvedDateFilter{
				Operator:     "between",
				Start:        format(start),
				End:          format(end),
				IncludeEmpty: value.IncludeEmpty,
			}
		case hasStart:
			return ResolvedDateFilter{Operator: "gte", Start: format(start), IncludeEmpty: value.IncludeEmpty}
		case hasEnd:
			return ResolvedDateFilter{Operator: "lte", End: format(end), IncludeEmpty: value.IncludeEmpty}
		default:
			return emptyResolution()
		}

	case "special":
		switch value.Value {
		case "empty":
			return ResolvedDateFilter{Operator: "isNull"}
		case "notEmpty":
			return ResolvedDateFilter{Operator: "isNotNull"}
		default:
			return emptyResolution()
		}

	case "single":
		date, ok := parseDateValue(value.Date)
		if !ok {
			return emptyResolution()
		}
		var start, end time.Time
		switch granularity {
		case DateGranularityMonth:
			start, end = startOfMonth(date), endOfMonth(date)
		case DateGranularityWeek:
			start, end = startOfWeek(date, ws), endOfWeek(date, ws)
		case DateGranularityHour:
			start, end = startOfHour(date), endOfHour(date)
		default:
			start, end = startOfDay(date), endOfDay(date)
		}
		return ResolvedDateFilter{Operator: "between", Start: format(start), End: format(end)}

	default:
		return emptyResolution()
	}
}

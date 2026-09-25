// Package columnids 把落库的列引用从「列名」反写为「列的稳定 id」。
//
// 为什么需要：
//   - 数据集列的引用键自本特性起是 `DatasetColumn.id`（后端分配的短 ID），列名
//     （`DatasetColumn.name`）降级为可变展示名——改名不应切断已保存的引用。
//   - 历史数据里 `bi_chart.config` 的 `bindings[].field` / `filters[].field` 与
//     `bi_dataset.shard_keys` 存的都是列名，必须一次性改写。
//
// 迁移做三件事（幂等，可重复执行）：
//  1. 逐个数据集物化列（缺 id 的补发），顺带把历史遗留的「带反引号表达式」
//     （`` `col` ``）归一为裸列名——反引号在 PostgreSQL 下会原样渲染成语法错误，
//     且会让「物理列 = expr 恰为列名本身」的判定失效。
//  2. 改写该数据集所有图表的 v2 配置：字段引用键 field → fieldId，值按 name→id 换成列 ID。
//  3. 按 name→id 改写 shard_keys。
//
// 明确不改的东西：无 version 的旧结构图表（用 `fields: []` 位置 id）保持原样——
// 位置 id 只有在运行时字段列表（带顺序）的上下文里才能解析，那是前端打开图表时
// 就地迁移的职责（`migrateChartConfig`）。本包只把它们计入 skipped 供人工核对。
package columnids

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"data-insights/internal/domain/entity"
	"data-insights/internal/service/dataset"

	"github.com/uptrace/bun"
)

// Report 是迁移结果摘要，供命令行输出。
type Report struct {
	Datasets           int
	ColumnsLoaded      int // 成功取到列（并完成物化）的数据集数
	ExprNormalized     int // 表达式被归一的列数
	ShardKeysRewritten int
	ChartsScanned      int
	ChartsRewritten    int
	Skipped            []string // 无法安全改写、留给前端懒迁移或需人工处理的对象
	Unresolved         []string // 引用了不存在列的字段引用（改不了，保持原值）
	Failures           []string // 数据集列读取失败（如数据源不可达）
}

// String 输出人类可读的摘要。
func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "数据集 %d 个（成功物化 %d 个 · 归一表达式 %d 处）\n", r.Datasets, r.ColumnsLoaded, r.ExprNormalized)
	fmt.Fprintf(&b, "shard_keys 改写 %d 个数据集\n", r.ShardKeysRewritten)
	fmt.Fprintf(&b, "图表扫描 %d 个 · 改写 %d 个\n", r.ChartsScanned, r.ChartsRewritten)
	writeList(&b, "跳过（旧结构，留待前端打开时迁移）", r.Skipped)
	writeList(&b, "未能解析的字段引用（保持原值）", r.Unresolved)
	writeList(&b, "数据集列读取失败", r.Failures)
	return b.String()
}

func writeList(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s（%d）:\n", title, len(items))
	for _, item := range items {
		fmt.Fprintf(b, "  - %s\n", item)
	}
}

// chartRow 是迁移只需要读的图表字段子集。
type chartRow struct {
	bun.BaseModel `bun:"bi_chart"`

	ID        int            `bun:"id,pk"`
	DatasetID int            `bun:"dataset_id"`
	Config    sql.NullString `bun:"config"`
}

// Run 执行迁移。apply 为 false 时只扫描与统计，**不改写任何引用**。
//
// 注意：无论 apply 与否，第 1 步（补发列 ID）都会落库——它只是给列新增 `id` 字段，
// 不改动任何既有引用的语义，幂等且可安全重跑；把它也纳入 dry-run 的禁写范围会让
// "先看看会改什么" 失去意义（没有 ID 就无从判断引用能否解析）。
func Run(ctx context.Context, db *bun.DB, securityKey []byte, apply bool) (*Report, error) {
	report := &Report{}

	svc := dataset.NewService(db)
	svc.SetSecurityKey(securityKey)

	datasets, err := svc.List(ctx, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("list datasets: %w", err)
	}
	report.Datasets = len(datasets)

	// 每个数据集的列引用映射：列名 → 列 ID，以及已知列 ID 集合（用于识别「已迁移」）
	nameToID := make(map[int]map[string]string, len(datasets))
	knownIDs := make(map[int]map[string]bool, len(datasets))
	datasetNames := make(map[int]string, len(datasets))

	for _, ds := range datasets {
		datasetNames[ds.ID] = ds.Name

		columns, err := svc.GetColumns(ctx, ds.ID)
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("dataset=%d(%s): %v", ds.ID, ds.Name, err))
			continue
		}

		normalized := 0
		for i := range columns {
			// 历史遗留：create 流程曾把物理列表达式写成 `col`（带反引号）
			if trimmed := strings.TrimSpace(columns[i].Expr); len(trimmed) > 1 &&
				strings.HasPrefix(trimmed, "`") && strings.HasSuffix(trimmed, "`") &&
				strings.Trim(trimmed, "`") == columns[i].Name {
				columns[i].Expr = columns[i].Name
				normalized++
			}
		}
		if normalized > 0 {
			if apply {
				if _, err := svc.UpdateColumns(ctx, ds.ID, columns); err != nil {
					report.Failures = append(report.Failures, fmt.Sprintf("dataset=%d(%s) 回写列失败: %v", ds.ID, ds.Name, err))
					continue
				}
			}
			report.ExprNormalized += normalized
		}

		nameToID[ds.ID] = make(map[string]string, len(columns))
		knownIDs[ds.ID] = make(map[string]bool, len(columns))
		for _, column := range columns {
			if column.ID == "" {
				continue
			}
			// 重名列是脏数据，取先出现者（与后端写入口的唯一性校验一致）
			if _, exists := nameToID[ds.ID][column.Name]; !exists {
				nameToID[ds.ID][column.Name] = column.ID
			}
			knownIDs[ds.ID][column.ID] = true
		}
		report.ColumnsLoaded++
	}

	// shard_keys：改写为列 ID
	report.ShardKeysRewritten = rewriteShardKeys(ctx, db, datasets, nameToID, knownIDs, report, apply)

	// 图表配置：改写 v2 文档里的字段引用
	if err := rewriteCharts(ctx, db, nameToID, knownIDs, datasetNames, report, apply); err != nil {
		return report, err
	}

	return report, nil
}

// rewriteShardKeys 把 shard_keys 里的列名换成列 ID。已经是 ID 的原样保留；
// 不在该数据集列集合里的条目保持原值并计入未解析。
func rewriteShardKeys(
	ctx context.Context,
	db *bun.DB,
	datasets []entity.Dataset,
	nameToID map[int]map[string]string,
	knownIDs map[int]map[string]bool,
	report *Report,
	apply bool,
) int {
	rewritten := 0
	for _, ds := range datasets {
		if strings.TrimSpace(ds.ShardKeys) == "" {
			continue
		}
		var keys []string
		if err := json.Unmarshal([]byte(ds.ShardKeys), &keys); err != nil {
			continue
		}
		changed := false
		for i, key := range keys {
			if id, ok := nameToID[ds.ID][key]; ok {
				keys[i] = id
				changed = true
				continue
			}
			if !knownIDs[ds.ID][key] {
				report.Unresolved = append(report.Unresolved, fmt.Sprintf("dataset=%d(%s) shard_key=%q", ds.ID, ds.Name, key))
			}
		}
		if !changed {
			continue
		}
		encoded, err := json.Marshal(keys)
		if err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("dataset=%d(%s) shard_keys 序列化失败: %v", ds.ID, ds.Name, err))
			continue
		}
		if apply {
			if _, err := db.NewUpdate().
				Table("bi_dataset").
				Set("shard_keys = ?", string(encoded)).
				Where("id = ?", ds.ID).
				Exec(ctx); err != nil {
				report.Failures = append(report.Failures, fmt.Sprintf("dataset=%d(%s) shard_keys 回写失败: %v", ds.ID, ds.Name, err))
				continue
			}
		}
		rewritten++
	}
	return rewritten
}

// rewriteCharts 逐图改写 v2 配置：bindings[].field 与 filters[].field 改键名为
// fieldId，并把值从列名换成列 ID。
func rewriteCharts(
	ctx context.Context,
	db *bun.DB,
	nameToID map[int]map[string]string,
	knownIDs map[int]map[string]bool,
	datasetNames map[int]string,
	report *Report,
	apply bool,
) error {
	var rows []chartRow
	if err := db.NewSelect().
		Table("bi_chart").
		Column("id", "dataset_id", "config").
		Where("deleted_at IS NULL").
		Scan(ctx, &rows); err != nil {
		return fmt.Errorf("scan charts: %w", err)
	}
	report.ChartsScanned = len(rows)

	for _, row := range rows {
		label := fmt.Sprintf("chart=%d(dataset=%d %s)", row.ID, row.DatasetID, datasetNames[row.DatasetID])
		rewritten, changed, unresolved, skipped := translateConfig(
			row.Config.String,
			nameToID[row.DatasetID],
			knownIDs[row.DatasetID],
		)
		for _, ref := range unresolved {
			report.Unresolved = append(report.Unresolved, fmt.Sprintf("%s field=%q", label, ref))
		}
		if skipped != "" {
			report.Skipped = append(report.Skipped, fmt.Sprintf("%s %s", label, skipped))
			continue
		}
		if !changed {
			continue
		}
		if apply {
			if _, err := db.NewUpdate().
				Table("bi_chart").
				Set("config = ?", rewritten).
				Where("id = ?", row.ID).
				Exec(ctx); err != nil {
				report.Failures = append(report.Failures, fmt.Sprintf("%s 回写失败: %v", label, err))
				continue
			}
		}
		report.ChartsRewritten++
	}
	return nil
}

// translateConfig 把 v2 图表配置里的字段引用从列名改写为列 ID。
// 返回改写后的 JSON、是否有改动、未解析的引用，以及跳过原因（非空表示未处理）。
//
// 只认 `query` 键下的 v2 形状；旧结构（`queryConfig` / `fields[]` 位置 id）由前端
// 打开图表时就地迁移——位置 id 的解析依赖运行时字段顺序，这里是拿不到的。
func translateConfig(raw string, nameToID map[string]string, knownIDs map[string]bool) (string, bool, []string, string) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil, ""
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, false, nil, "配置 JSON 损坏"
	}
	query, ok := doc["query"].(map[string]any)
	if !ok {
		return raw, false, nil, "旧结构（无 query 小节）"
	}

	var unresolved []string
	changed := false

	// 持久化键定稿为 fieldId（与文档内既有的 bindingId / chartType 同为 camelCase）。
	// 旧键 field 里存的值有两代：本迁移之前是列名，之后是列 ID——统一搬进新键，
	// 能解析成列 ID 的顺带换值，悬空引用保留原值并计入未解析。
	translate := func(container map[string]any) {
		field, ok := container["field"].(string)
		if !ok || field == "" {
			return
		}
		delete(container, "field")
		if id, ok := nameToID[field]; ok {
			container["fieldId"] = id
		} else if knownIDs[field] {
			container["fieldId"] = field // 已经是列 ID，只需改键名
		} else {
			container["fieldId"] = field
			unresolved = append(unresolved, field)
		}
		changed = true
	}

	for _, groupKey := range []string{"dimensionGroups", "metricGroups"} {
		groups, _ := query[groupKey].([]any)
		for _, group := range groups {
			groupMap, ok := group.(map[string]any)
			if !ok {
				continue
			}
			bindings, _ := groupMap["bindings"].([]any)
			for _, binding := range bindings {
				if bindingMap, ok := binding.(map[string]any); ok {
					translate(bindingMap)
				}
			}
		}
	}
	if filters, ok := query["filters"].([]any); ok {
		for _, filter := range filters {
			if filterMap, ok := filter.(map[string]any); ok {
				translate(filterMap)
			}
		}
	}

	if !changed {
		return raw, false, unresolved, ""
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw, false, unresolved, "改写后序列化失败"
	}
	return string(out), true, unresolved, ""
}

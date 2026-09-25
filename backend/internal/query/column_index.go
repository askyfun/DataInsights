package query

// 列索引：把 bi_dataset.columns（JSON 文档）编译成查询期需要的三张表。
//
// 为什么要三张表：
//   - 权威引用是**列 ID**——bi_chart.config 的 bindings[].columnId、bi_dataset.shard_keys、
//     以及前端发来的查询请求（dims/metrics/filters）都只带列 ID。列名是可变的展示名，
//     改一次名不该打断任何已保存的图表，所以 ID 才是解析入口。
//   - SQL 输出别名要用**列名**（人读友好，分享页与仪表盘不需要字段列表就能读负载），
//     所以需要 id→name。
//   - byName 是**过渡态次级索引**：部署前保存的历史配置里 field 存的还是列名。
//     一次性反写迁移跑完后它不会再被命中；命中时调用方会记警告，便于确认可以删除。

// ColumnInfo 是 bi_dataset.columns JSON 文档里的单个列定义（列 ID 为权威标识）。
type ColumnInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Expr string `json:"expr"`
	Role string `json:"role"`
	Type string `json:"type"`
}

// columnIndex 是字段引用的解析索引。
type columnIndex struct {
	// byID 列 ID → SQL 表达式（权威）
	byID map[string]string
	// nameByID 列 ID → 展示名（用于把默认输出别名解析回列名）
	nameByID map[string]string
	// byName 列名 → SQL 表达式（过渡态次级索引，命中即说明配置尚未迁移）
	byName map[string]string
}

func newColumnIndex() columnIndex {
	return columnIndex{
		byID:     make(map[string]string),
		nameByID: make(map[string]string),
		byName:   make(map[string]string),
	}
}

// buildColumnIndex 解析 bi_dataset.columns。
// 空串与空数组返回空索引（列未物化时按裸名透传，与改动前一致）；JSON 损坏返回错误。
func buildColumnIndex(columns string) (columnIndex, error) {
	idx := newColumnIndex()
	if columns == "" {
		return idx, nil
	}

	var cols []ColumnInfo
	if err := unmarshalJSON(columns, &cols); err != nil {
		return idx, err
	}

	for _, col := range cols {
		if col.Expr == "" {
			continue
		}
		if col.ID != "" {
			idx.byID[col.ID] = col.Expr
			if col.Name != "" {
				idx.nameByID[col.ID] = col.Name
			}
		}
		// 同名取先出现者：重名列是脏数据，后端写入口已校验唯一性
		if col.Name != "" {
			if _, exists := idx.byName[col.Name]; !exists {
				idx.byName[col.Name] = col.Expr
			}
		}
	}
	return idx, nil
}

// expr 解析字段标识为 SQL 表达式，返回是否由权威索引（列 ID）命中。
// 未命中任何索引时原样返回入参（裸名透传，既有兜底行为）。
func (idx columnIndex) expr(field string) (string, bool) {
	if e, ok := idx.byID[field]; ok && e != "" {
		return e, true
	}
	if e, ok := idx.byName[field]; ok && e != "" {
		return e, false
	}
	return field, false
}

// displayName 返回列 ID 对应的展示名。
func (idx columnIndex) displayName(field string) (string, bool) {
	name, ok := idx.nameByID[field]
	return name, ok && name != ""
}

// localizeFields 把字段标识列表从列 ID 换成展示名（列名）。
// 未命中列 ID 的条目原样保留——请求在 wire 上已经是列名（历史形态或仅按名引用的
// 场景）时不做任何改动，行为与改动前完全一致。
//
// 调用场景：executor 在拿到 dataset columns 后，把请求携带的维度列表翻译成
// 「SQL 输出别名 / 响应负载键」使用的列名，供分页表格与各处理器组装负载时对齐。
func (idx columnIndex) localizeFields(fields []string) []string {
	if len(idx.nameByID) == 0 || len(fields) == 0 {
		return fields
	}
	out := make([]string, len(fields))
	for i, field := range fields {
		if name, ok := idx.displayName(field); ok {
			out[i] = name
			continue
		}
		out[i] = field
	}
	return out
}

// localizeAlias 把指标别名从列 ID 换成展示名。显式别名（与字段标识不同）保持不动。
// 等价于 displayAlias(field, alias, "")——指标没有时间粒度后缀，单独留个名字是为了
// 让 executor 侧的调用点读起来不出现裸空串参数。
func (idx columnIndex) localizeAlias(field, alias string) string {
	return idx.displayAlias(field, alias, "")
}

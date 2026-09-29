package query

import (
	"strings"
)

// topNGroupsAlias 是「未截断全量分组」汇总查询里组数的结果列名。只在 Go 端读取
// （executor 用它判断是否真的有剩余分组），不进任何响应。与 BuildCountQuery 的
// `_total`/`_count_query` 同款：下划线前缀字面量直接写，不过 safeIdentifier。
const topNGroupsAlias = "_topn_groups"

// BuildTopNRestQuery 为 Top N 的「其余合并为其他」（issue #116 验收行）构造一条
// 汇总查询：
//
//	SELECT COUNT(*) AS _topn_groups, SUM(<m1>) AS <m1>, SUM(<m2>) AS <m2>, ...
//	FROM ( <完整分组查询，不带 Sort/Limit> ) AS _topn_rest
//
// 一次往返同时给出「未截断的总分组数」与「各指标在全量分组上的可加重聚合」，
// 调用方用后者减去前 N 行之和即得「其他」值——与表格合计行（issue #131）同一条
// 「聚合交给数据库、Go 端不重算明细」的口径。
//
// 为什么是「全量 − 前 N」而不是「直接聚合剩余」：剩余是由排名定义的（第 N+1 名
// 往后），SQL 侧要拿到排名得用窗口函数，或在子查询里写 `OFFSET N`——后者在无 LIMIT
// 配套时各方言形态不一（MySQL 必须有 LIMIT），而本仓只有 PostgreSQL 做过真实能力
// 验证。派生表 + COUNT/SUM 是 BuildCountQuery 已经在四种方言上跑通的形状，新增
// SQL 面最小。
//
// 前提：指标全部可加（SUM/COUNT），由调用方经 topNAdditiveMetrics 门挡过——
// AVG 在这里会变成「平均数的平均数」、COUNT(DISTINCT) 会变成「跨组去重后求和」，
// MIN/MAX 更是无法从全量与前 N 反推（剩余段的 MIN 与全量 MIN 无关）。
//
// 入参 ast 允许已被 applyTopN 改过（Sort/Limit 已置）：内层查询是其**去掉
// Sort/Limit 的副本**，外层别名取自 ast.Metrics，与主查询的列名逐字一致。
// 参数化：过滤值经 BuildSelectQuery 的 buildWhereClause 走占位符，返回的 args 即
// 内层 WHERE 的参数；外层只做列引用，不引入新参数。
func BuildTopNRestQuery(dialect DialectType, ast *QueryAST) (string, []any) {
	qb := NewBunQueryBuilder()
	qb.SetDialect(dialect)
	qb.withASTIndex(ast)
	return qb.buildTopNRestQuery(ast)
}

// buildTopNRestQuery 是 BuildTopNRestQuery 的实现体，与通用路径共用 BunQueryBuilder
// 的指标/过滤渲染方法。
func (qb *BunQueryBuilder) buildTopNRestQuery(ast *QueryAST) (string, []any) {
	if len(ast.Metrics) == 0 {
		return "", nil // 没有指标就没有「其他」可算的量；调用方据此跳过
	}

	// 内层：去掉 Top N 写进 AST 的 Sort/Limit（以及分页），拿到未截断的全部分组。
	// 浅拷贝即可——builder 只读 Dimensions/Metrics/Filters 等切片，不回写。
	inner := *ast
	inner.Sort = nil
	inner.Limit = 0
	inner.Pagination = nil
	innerSQL, innerArgs := qb.BuildSelectQuery(&inner)

	var sb strings.Builder
	sb.WriteString("SELECT COUNT(*) AS ")
	sb.WriteString(topNGroupsAlias)
	for _, metric := range ast.Metrics {
		alias := qb.quoteResultAlias(metric.Alias)
		sb.WriteString(", SUM(")
		sb.WriteString(alias)
		sb.WriteString(") AS ")
		sb.WriteString(alias)
	}
	sb.WriteString(" FROM (")
	sb.WriteString(innerSQL)
	sb.WriteString(") AS _topn_rest")

	return sb.String(), innerArgs
}

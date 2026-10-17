// Package listpage is the pagination / sorting contract shared by the four
// resource list endpoints (datasources, datasets, charts, dashboards).
//
// The clamp semantics are deliberately the ones the handlers used before the
// move to server-side paging: a missing/garbage/out-of-range limit falls back
// to 100 instead of failing the request, and a negative offset becomes 0.
// Sorting is whitelist-only: an unknown field or direction falls back to the
// entity's default order, never into the SQL string.
package listpage

import (
	"strings"
)

const (
	// DefaultLimit is the page size used when the client omits a usable limit.
	DefaultLimit = 100
	// MaxLimit is the largest page a client may request; larger values clamp
	// back to DefaultLimit (matching the pre-existing behaviour).
	MaxLimit = 1000
)

// Params is the normalized list query. Limit == 0 means "no limit" and is
// reserved for internal callers (handlers always produce a positive limit).
type Params struct {
	Limit   int
	Offset  int
	Sort    string
	Order   string
	Keyword string
}

// New normalizes raw limit/offset values and carries sort/order/keyword
// through untouched (the services interpret them through OrderExpr).
func New(limit, offset int, sort, order, keyword string) Params {
	if limit <= 0 || limit > MaxLimit {
		limit = DefaultLimit
	}
	if offset < 0 {
		offset = 0
	}
	return Params{
		Limit:   limit,
		Offset:  offset,
		Sort:    strings.TrimSpace(sort),
		Order:   strings.TrimSpace(order),
		Keyword: strings.TrimSpace(keyword),
	}
}

// OrderExpr resolves a client sort request against an allow-list of
// column expressions. fallback is the entity's default ORDER BY (it may
// contain a tie-breaker such as "created_at DESC, id DESC"). Unknown fields
// and directions yield the fallback, so no client input reaches the SQL.
func OrderExpr(sort, order string, allowed map[string]string, fallback string) string {
	expr, ok := allowed[strings.ToLower(sort)]
	if !ok {
		return fallback
	}
	switch strings.ToLower(order) {
	case "asc":
		return expr + " ASC"
	case "desc":
		return expr + " DESC"
	default:
		return fallback
	}
}

// Contains turns a user keyword into a case-insensitive substring pattern,
// escaping LIKE wildcards so a literal % or _ does not widen the match.
func Contains(keyword string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(keyword)
	return "%" + escaped + "%"
}

// Result is the wire payload of every list endpoint: the page of items plus
// the total row count of the filtered set (COUNT, not len(items)).
type Result[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

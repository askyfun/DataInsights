package chart

import "data-insights/internal/domain/entity"

// applyFilterOverrides implements the dashboard filter merge (PRD §8.3, D5–D7).
//
// Given the chart's own resolved request plus the conditions a dashboard wants to
// impose, it returns a request whose filter list is:
//
//	base   = req.Filters whose field is NOT claimed by any override (kept as-is)
//	result = base ++ overrides            (AND-appended)
//
// i.e. "where the dashboard has a condition the dashboard wins, and the chart's
// own condition on that field is dropped entirely; everywhere else the chart's
// conditions stay in force".
//
// Two properties matter as much as the rule itself:
//   - The input request is NEVER mutated: the merged request is a copy, so a
//     caller that reuses `req` (e.g. a retry) still sees the original conditions.
//   - Nothing here touches bi_chart.config (D7). The override exists only for the
//     lifetime of this single query; the chart asset on disk is untouched.
//
// Overrides with an empty field are ignored (they cannot claim anything), and an
// empty override list short-circuits to the original request. Appended entries
// default to AND, so a dashboard can only narrow a chart, never widen it.
func applyFilterOverrides(
	req *entity.ChartQueryRequest, overrides []entity.Filter,
) *entity.ChartQueryRequest {
	if req == nil || len(overrides) == 0 {
		return req
	}

	claimed := make(map[string]struct{}, len(overrides))
	appended := make([]entity.Filter, 0, len(overrides))
	for _, override := range overrides {
		if override.Field == "" {
			continue
		}
		claimed[override.Field] = struct{}{}
		if override.Logic == "" {
			override.Logic = "and"
		}
		appended = append(appended, override)
	}
	if len(appended) == 0 {
		return req
	}

	kept := make([]entity.Filter, 0, len(req.Filters))
	for _, filter := range req.Filters {
		if _, hit := claimed[filter.Field]; hit {
			continue
		}
		kept = append(kept, filter)
	}

	merged := *req
	merged.Filters = append(kept, appended...)
	return &merged
}

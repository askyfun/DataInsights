package handler

import (
	"strconv"

	"data-insights/internal/listpage"
)

// listParams is the single binding→Params conversion shared by the four
// resource list endpoints. Raw values stay strings in the In structs so the
// lenient pre-existing fallbacks survive (garbage limit → 100, negative
// offset → 0) instead of turning into a 400 bind error.
func listParams(limit, offset, sort, order, keyword string) listpage.Params {
	l, _ := strconv.Atoi(orDefault(limit, "100"))
	o, _ := strconv.Atoi(orDefault(offset, "0"))
	return listpage.New(l, o, sort, order, keyword)
}

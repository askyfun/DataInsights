package listpage

import "testing"

func TestNewClampsLikeTheLegacyHandler(t *testing.T) {
	cases := []struct {
		limit, offset int
		wantLimit     int
		wantOffset    int
	}{
		{0, 0, DefaultLimit, 0},
		{-5, -3, DefaultLimit, 0},
		{MaxLimit + 1, 5, DefaultLimit, 5},
		{MaxLimit, 5, MaxLimit, 5},
		{10, 100, 10, 100},
	}
	for _, tc := range cases {
		got := New(tc.limit, tc.offset, "", "", "")
		if got.Limit != tc.wantLimit || got.Offset != tc.wantOffset {
			t.Errorf("New(%d,%d) = limit %d offset %d, want limit %d offset %d",
				tc.limit, tc.offset, got.Limit, got.Offset, tc.wantLimit, tc.wantOffset)
		}
	}
}

func TestOrderExprWhitelistsFieldAndDirection(t *testing.T) {
	allowed := map[string]string{"name": "name", "created_at": "created_at"}
	const fallback = "id DESC"

	if got := OrderExpr("name", "ASC", allowed, fallback); got != "name ASC" {
		t.Errorf("known field: got %q", got)
	}
	if got := OrderExpr("NAME", "desc", allowed, fallback); got != "name DESC" {
		t.Errorf("case-insensitive field: got %q", got)
	}
	// Unknown field / direction / injection attempt all fall back verbatim.
	for _, tc := range []struct{ sort, order string }{
		{"", ""},
		{"password", "asc"},
		{"name", "sideways"},
		{"name; DROP TABLE bi_dataset", "asc"},
	} {
		if got := OrderExpr(tc.sort, tc.order, allowed, fallback); got != fallback {
			t.Errorf("OrderExpr(%q,%q) = %q, want fallback %q", tc.sort, tc.order, got, fallback)
		}
	}
}

func TestContainsEscapesLikeWildcards(t *testing.T) {
	if got := Contains("a%b_c\\d"); got != `%a\%b\_c\\d%` {
		t.Errorf("Contains escaping wrong: %q", got)
	}
}

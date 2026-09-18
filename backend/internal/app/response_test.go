package app

import "testing"

func TestNewPageResultHasMore(t *testing.T) {
	cases := []struct {
		total, page, limit int
		wantHasMore        bool
	}{
		{total: 455, page: 1, limit: 100, wantHasMore: true},
		{total: 455, page: 5, limit: 100, wantHasMore: false},
		{total: 100, page: 1, limit: 100, wantHasMore: false},
		{total: 0, page: 1, limit: 100, wantHasMore: false},
		{total: 101, page: 1, limit: 100, wantHasMore: true},
	}
	for _, c := range cases {
		got := newPageResult([]Word{}, c.total, c.page, c.limit)
		if got.HasMore != c.wantHasMore {
			t.Fatalf("total=%d page=%d limit=%d: HasMore = %v, want %v", c.total, c.page, c.limit, got.HasMore, c.wantHasMore)
		}
	}
}

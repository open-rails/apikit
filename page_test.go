package apikit_test

import (
	"testing"

	"github.com/open-rails/apikit"
)

func TestListParamsNormalize(t *testing.T) {
	cases := []struct {
		name                   string
		in                     apikit.ListParams
		defaultLimit, maxLimit int
		wantLimit, wantOffset  int
	}{
		{"zero limit takes the default", apikit.ListParams{}, 20, 100, 20, 0},
		{"negative limit takes the default", apikit.ListParams{Limit: -5}, 20, 100, 20, 0},
		{"limit is capped", apikit.ListParams{Limit: 5000}, 20, 100, 100, 0},
		{"negative offset floors at zero", apikit.ListParams{Limit: 10, Offset: -3}, 20, 100, 10, 0},
		{"a valid page is untouched", apikit.ListParams{Limit: 50, Offset: 100}, 20, 100, 50, 100},
		{"a zero cap means uncapped", apikit.ListParams{Limit: 5000}, 20, 0, 5000, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.in
			p.Normalize(tc.defaultLimit, tc.maxLimit)
			if p.Limit != tc.wantLimit || p.Offset != tc.wantOffset {
				t.Errorf("got limit=%d offset=%d, want limit=%d offset=%d", p.Limit, p.Offset, tc.wantLimit, tc.wantOffset)
			}
		})
	}
}

func TestHasMore(t *testing.T) {
	if !apikit.HasMore(0, 20, 21) {
		t.Error("21 rows past a first page of 20 has more")
	}
	if apikit.HasMore(0, 20, 20) {
		t.Error("exactly one page has no more")
	}
	if apikit.HasMore(20, 20, 20) {
		t.Error("past the end has no more")
	}
}

func TestHasMoreFromLen(t *testing.T) {
	if !apikit.HasMoreFromLen(0, 20, 21) {
		t.Error("want more")
	}
	if apikit.HasMoreFromLen(10, 5, 15) {
		t.Error("a short page reaching the total has no more")
	}
}

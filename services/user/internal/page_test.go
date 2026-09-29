package internal

import (
	"net/http/httptest"
	"testing"
)

func TestPageClampsLimitAndOffset(t *testing.T) {
	cases := map[string][2]int{
		"":                   {50, 0},
		"?limit=10&offset=5": {10, 5},
		"?limit=500":         {200, 0},
		"?limit=0":           {1, 0},
		"?limit=x&offset=-3": {50, 0},
		"?offset=999999999":  {50, 100_000},
	}
	for qs, want := range cases {
		l, o := page(httptest.NewRequest("GET", "/"+qs, nil))
		if l != want[0] || o != want[1] {
			t.Errorf("%q: got (%d, %d), want (%d, %d)", qs, l, o, want[0], want[1])
		}
	}
}

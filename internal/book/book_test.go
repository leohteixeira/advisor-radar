package book_test

import (
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/book"
)

func TestSegmentFromAssets_Bounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		assets float64
		want   string
	}{
		{name: "essencial_at_bound", assets: 10_000, want: book.SegmentEssencial},
		{name: "advance_just_above", assets: 10_000.01, want: book.SegmentAdvance},
		{name: "advance_at_bound", assets: 200_000, want: book.SegmentAdvance},
		{name: "singular_just_above", assets: 200_000.01, want: book.SegmentSingular},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := book.SegmentFromAssets(tc.assets); got != tc.want {
				t.Fatalf("SegmentFromAssets(%v) = %q, want %q", tc.assets, got, tc.want)
			}
		})
	}
}

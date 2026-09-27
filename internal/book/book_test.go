package book_test

import (
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/book"
)

func TestClients_SegmentMatchesAUM(t *testing.T) {
	t.Parallel()

	if len(book.Clients) != 22 {
		t.Fatalf("Clients len = %d, want 22", len(book.Clients))
	}

	for id, c := range book.Clients {
		got := book.SegmentFromAssets(c.AUM)
		if got != c.Segment {
			t.Errorf("%s: SegmentFromAssets(%.0f) = %q, want %q", id, c.AUM, got, c.Segment)
		}
		if c.ID != id {
			t.Errorf("Clients[%s].ID = %q, want %q", id, c.ID, id)
		}
	}
}

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

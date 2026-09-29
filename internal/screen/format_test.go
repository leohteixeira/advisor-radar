package screen

import (
	"math"
	"slices"
	"testing"
	"time"
)

func TestMoney(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cents    int64
		expected string
	}{
		{name: "zero", cents: 0, expected: "US$ 0,00"},
		{name: "cents only", cents: 5, expected: "US$ 0,05"},
		{name: "one dollar", cents: 100, expected: "US$ 1,00"},
		{name: "grouping", cents: 123_456, expected: "US$ 1.234,56"},
		{name: "thiago patrimony", cents: 6_800_000, expected: "US$ 68.000,00"},
		{name: "thiago cash", cents: 6_052_000, expected: "US$ 60.520,00"},
		{name: "mariana patrimony", cents: 24_830_000, expected: "US$ 248.300,00"},
		{name: "millions", cents: 123_456_789_01, expected: "US$ 123.456.789,01"},
		{name: "negative uses u+2212", cents: -3_850_200, expected: "− US$ 38.502,00"},
		{name: "min int64 does not overflow", cents: math.MinInt64, expected: "− US$ 92.233.720.368.547.758,08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Money(tt.cents); got != tt.expected {
				t.Errorf("Money(%d) = %q, want %q", tt.cents, got, tt.expected)
			}
		})
	}
}

func TestPercent(t *testing.T) {
	t.Parallel()
	if got := Percent(62); got != "62%" {
		t.Errorf("Percent(62) = %q, want %q", got, "62%")
	}
}

func TestShares(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		values   []int64
		expected []int
	}{
		{name: "thiago", values: []int64{204_000, 544_000, 6_052_000}, expected: []int{3, 8, 89}},
		{name: "fernanda", values: []int64{164_000, 369_000, 172_200, 114_800}, expected: []int{20, 45, 21, 14}},
		{name: "mariana largest remainders", values: []int64{9_090_000, 6_060_000, 3_680_000, 6_000_000}, expected: []int{37, 24, 15, 24}},
		{name: "thirds tie to earlier", values: []int64{1, 1, 1}, expected: []int{34, 33, 33}},
		{name: "single", values: []int64{42}, expected: []int{100}},
		{name: "all zero", values: []int64{0, 0}, expected: []int{0, 0}},
		{name: "negative counts as zero", values: []int64{-5, 10}, expected: []int{0, 100}},
		{name: "empty", values: []int64{}, expected: []int{}},
		{name: "huge values stay exact", values: []int64{math.MaxInt64, math.MaxInt64}, expected: []int{50, 50}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Shares(tt.values)
			if err != nil {
				t.Fatalf("Shares(%v) error = %v", tt.values, err)
			}
			if !slices.Equal(got, tt.expected) {
				t.Errorf("Shares(%v) = %v, want %v", tt.values, got, tt.expected)
			}
		})
	}
}

func TestShares_SumIsHundred(t *testing.T) {
	t.Parallel()
	values := []int64{7, 13, 29, 51, 101, 3}
	got, err := Shares(values)
	if err != nil {
		t.Fatalf("Shares error = %v", err)
	}
	sum := 0
	for _, s := range got {
		sum += s
	}
	if sum != 100 {
		t.Errorf("sum of %v = %d, want 100", got, sum)
	}
}

func TestShares_Overflow(t *testing.T) {
	t.Parallel()
	if _, err := Shares([]int64{math.MaxInt64, math.MaxInt64, math.MaxInt64}); err == nil {
		t.Error("Shares of a sum past uint64 returned no error")
	}
}

func TestRelativeTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		age      time.Duration
		expected string
	}{
		{name: "negative", age: -time.Minute, expected: "agora"},
		{name: "seconds", age: 59 * time.Second, expected: "agora"},
		{name: "minutes", age: 5 * time.Minute, expected: "há 5 min"},
		{name: "last minute of the hour", age: 59*time.Minute + 59*time.Second, expected: "há 59 min"},
		{name: "hours", age: 3*time.Hour + 20*time.Minute, expected: "há 3 h"},
		{name: "yesterday", age: 30 * time.Hour, expected: "ontem"},
		{name: "days", age: 4*24*time.Hour + time.Hour, expected: "há 4 dias"},
		{name: "two days", age: 48 * time.Hour, expected: "há 2 dias"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RelativeTime(tt.age); got != tt.expected {
				t.Errorf("RelativeTime(%v) = %q, want %q", tt.age, got, tt.expected)
			}
		})
	}
}

func TestFirstName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "two words", input: "Thiago Azevedo", expected: "Thiago"},
		{name: "padded", input: "  Mariana   Costa ", expected: "Mariana"},
		{name: "empty", input: "", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := FirstName(tt.input); got != tt.expected {
				t.Errorf("FirstName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestInitials(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "first two words", input: "Ana Paula Ribeiro", expected: "AP"},
		{name: "connector skipped", input: "Maria da Silva", expected: "MS"},
		{name: "lower case upper-cased", input: "élida rocha", expected: "ÉR"},
		{name: "one word", input: "Ana", expected: "A"},
		{name: "empty", input: "", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Initials(tt.input); got != tt.expected {
				t.Errorf("Initials(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

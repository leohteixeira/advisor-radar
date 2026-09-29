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

func TestCompactMoney(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cents    int64
		expected string
	}{
		{name: "whole dollars drop the cents", cents: 100_000, expected: "US$ 1.000"},
		{name: "ten dollars", cents: 1_000, expected: "US$ 10"},
		{name: "zero", cents: 0, expected: "US$ 0"},
		{name: "cents stay", cents: 1_250, expected: "US$ 12,50"},
		{name: "negative whole", cents: -5_000, expected: "− US$ 50"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := CompactMoney(tt.cents); got != tt.expected {
				t.Errorf("CompactMoney(%d) = %q, want %q", tt.cents, got, tt.expected)
			}
		})
	}
}

func TestSignedMoney(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cents    int64
		expected string
	}{
		{name: "gain", cents: 1_940_000, expected: "+US$ 19.400,00"},
		{name: "loss uses u+2212 and no space", cents: -25_000, expected: "−US$ 250,00"},
		{name: "zero has no sign", cents: 0, expected: "US$ 0,00"},
		{name: "one cent", cents: 1, expected: "+US$ 0,01"},
		{name: "min int64", cents: math.MinInt64, expected: "−US$ 92.233.720.368.547.758,08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SignedMoney(tt.cents); got != tt.expected {
				t.Errorf("SignedMoney(%d) = %q, want %q", tt.cents, got, tt.expected)
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

func TestChangePercent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		delta    int64
		base     int64
		expected string
	}{
		{name: "mariana return", delta: 1_940_000, base: 16_890_000, expected: "+11,5%"},
		{name: "position return", delta: 1_200_000, base: 6_000_000, expected: "+20,0%"},
		{name: "loss", delta: -535, base: 1_000, expected: "−53,5%"},
		{name: "half rounds away from zero", delta: 1, base: 2_000, expected: "+0,1%"},
		{name: "negative half rounds away from zero", delta: -1, base: 2_000, expected: "−0,1%"},
		{name: "below half rounds down", delta: 1, base: 2_001, expected: "+0,0%"},
		{name: "grouped thousands", delta: 1_234_500, base: 100, expected: "+1.234.500,0%"},
		{name: "zero delta", delta: 0, base: 1_000, expected: "0,0%"},
		{name: "no base", delta: 500, base: 0, expected: "0,0%"},
		{name: "negative base", delta: 500, base: -10, expected: "0,0%"},
		{name: "extreme values stay exact", delta: math.MinInt64, base: 1, expected: "−922.337.203.685.477.580.800,0%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ChangePercent(tt.delta, tt.base); got != tt.expected {
				t.Errorf("ChangePercent(%d, %d) = %q, want %q", tt.delta, tt.base, got, tt.expected)
			}
		})
	}
}

func TestSignTone(t *testing.T) {
	t.Parallel()
	for n, expected := range map[int64]string{1: TonePos, -1: ToneNeg, 0: ToneNeutral} {
		if got := SignTone(n); got != expected {
			t.Errorf("SignTone(%d) = %q, want %q", n, got, expected)
		}
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

func TestDays(t *testing.T) {
	t.Parallel()
	for n, expected := range map[int]string{1: "1 dia", 2: "2 dias", 4: "4 dias", 30: "30 dias"} {
		if got := Days(n); got != expected {
			t.Errorf("Days(%d) = %q, want %q", n, got, expected)
		}
	}
}

func TestProtocol(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"01a0e3a5-2f4c-7b1e-9d2a-5c6f7e8a9b0c": "01A0E3A5-2F4C",
		"01A0E3A5-2F4C-7B1E-9D2A-5C6F7E8A9B0C": "01A0E3A5-2F4C",
		"01a0e3a5-2f4c":                        "01A0E3A5-2F4C",
		"abc":                                  "ABC",
		"":                                     "",
	}
	for id, expected := range tests {
		if got := Protocol(id); got != expected {
			t.Errorf("Protocol(%q) = %q, want %q", id, got, expected)
		}
	}
}

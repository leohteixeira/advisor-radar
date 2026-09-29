package bff

import (
	"errors"
	"testing"
)

func TestParseSchemaRange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		value    string
		expected schemaRange
		invalid  bool
	}{
		{name: "one version", value: "1", expected: schemaRange{Min: 1, Max: 1}},
		{name: "range", value: "1-2", expected: schemaRange{Min: 1, Max: 2}},
		{name: "range of one", value: "3-3", expected: schemaRange{Min: 3, Max: 3}},
		{name: "future version", value: "2", expected: schemaRange{Min: 2, Max: 2}},
		{name: "surrounding whitespace", value: " \t1-2 ", expected: schemaRange{Min: 1, Max: 2}},
		{name: "leading zero", value: "01", expected: schemaRange{Min: 1, Max: 1}},
		{name: "reversed range", value: "2-1", invalid: true},
		{name: "letters", value: "abc", invalid: true},
		{name: "zero", value: "0", invalid: true},
		{name: "range from zero", value: "0-1", invalid: true},
		{name: "empty", value: "", invalid: true},
		{name: "only whitespace", value: "  ", invalid: true},
		{name: "open end", value: "1-", invalid: true},
		{name: "open start", value: "-1", invalid: true},
		{name: "plus sign", value: "+1", invalid: true},
		{name: "three bounds", value: "1-2-3", invalid: true},
		{name: "inner space", value: "1 - 2", invalid: true},
		{name: "list", value: "1,2", invalid: true},
		{name: "decimal", value: "1.0", invalid: true},
		{name: "overflow", value: "99999999999999999999", invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSchemaRange(tt.value)
			if tt.invalid {
				if !errors.Is(err, errSchemaRange) {
					t.Errorf("parseSchemaRange(%q) = %+v, %v; want %v", tt.value, got, err, errSchemaRange)
				}
				return
			}
			if err != nil || got != tt.expected {
				t.Errorf("parseSchemaRange(%q) = %+v, %v; want %+v", tt.value, got, err, tt.expected)
			}
		})
	}
}

func TestSchemaRange_Contains(t *testing.T) {
	t.Parallel()
	r := schemaRange{Min: 1, Max: 2}
	for v, expected := range map[int]bool{0: false, 1: true, 2: true, 3: false} {
		if got := r.contains(v); got != expected {
			t.Errorf("%+v contains %d = %t, want %t", r, v, got, expected)
		}
	}
}

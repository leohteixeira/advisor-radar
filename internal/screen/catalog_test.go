package screen

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"text/template"
)

func embedded(t *testing.T) Catalog {
	t.Helper()
	cat, err := parseCatalog(catalogJSON)
	if err != nil {
		t.Fatalf("embedded catalog: %v", err)
	}
	return cat
}

func TestParseCatalog_Embedded(t *testing.T) {
	t.Parallel()
	cat := embedded(t)
	if cat.Version() != 1 {
		t.Errorf("version = %d, want 1", cat.Version())
	}
	home, ok := cat.screens["home"]
	if !ok {
		t.Fatal("catalog has no home screen")
	}
	if home.revision != "v1" {
		t.Errorf("home revision = %q, want v1", home.revision)
	}
	var ids, types []string
	for _, s := range home.sections {
		ids = append(ids, s.id)
		types = append(types, s.typ)
	}
	if want := []string{"moment", "wealth", "actions", "advisor", "activity"}; !slices.Equal(ids, want) {
		t.Errorf("home sections = %v, want %v", ids, want)
	}
	if want := []string{"moment_card", "wealth_summary", "action_grid", "advisor_card", "activity_list"}; !slices.Equal(types, want) {
		t.Errorf("home types = %v, want %v", types, want)
	}
	investir, ok := cat.screens["investir"]
	if !ok {
		t.Fatal("catalog has no investir screen")
	}
	ids, types = nil, nil
	for _, s := range investir.sections {
		ids = append(ids, s.id)
		types = append(types, s.typ+"/"+strings.Join(s.variants, ","))
	}
	if want := []string{"cash", "highlights", "fixed_income", "etfs", "stocks"}; investir.revision != "v1" || !slices.Equal(ids, want) {
		t.Errorf("investir %s sections = %v, want v1 %v", investir.revision, ids, want)
	}
	if want := []string{
		"invest_summary/default",
		"product_rail/profile_conservador,profile_moderado,profile_arrojado",
		"product_list/fixed_income",
		"product_list/etf",
		"product_list/stocks",
	}; !slices.Equal(types, want) {
		t.Errorf("investir variants = %v, want %v", types, want)
	}
	if len(cat.screens) != 2 {
		t.Errorf("catalog serves %d screens, want home and investir", len(cat.screens))
	}
}

// TestCatalog_EveryTemplateExecutes parses and executes every copy and
// heading template, with every field set and with none, as a template that
// names a field Fields lacks fails only when executed.
func TestCatalog_EveryTemplateExecutes(t *testing.T) {
	t.Parallel()
	cat := embedded(t)
	full := Fields{
		FirstName: "Thiago", AdvisorName: "Ana Paula Ribeiro", Segment: "Advance", Since: "2024", SLA: "4 h",
		Gap: "US$ 1.800,00", Threshold: "US$ 10.000,00", Cash: "US$ 60.520,00", CashShare: "89%", IdleDays: "4 dias", Profile: "arrojado",
		Protocol: "01A0E3A5-2F4C", Age: "há 3 min", Risk: "5", MaxRisk: "2", Minimum: "US$ 10",
	}
	for key, tmpl := range cat.copy {
		for _, f := range []Fields{full, {}} {
			out, err := execute(tmpl, f)
			if err != nil {
				t.Errorf("%s/%s/%s: %v", key.typ, key.variant, key.key, err)
				continue
			}
			if strings.Contains(out, "<no value>") {
				t.Errorf("%s/%s/%s rendered %q", key.typ, key.variant, key.key, out)
			}
		}
		if out, _ := execute(tmpl, full); strings.TrimSpace(out) == "" {
			t.Errorf("%s/%s/%s renders empty with every field set", key.typ, key.variant, key.key)
		}
	}
	for slug, def := range cat.screens {
		for _, tmpl := range []*template.Template{def.title, def.subtitle} {
			if _, err := execute(tmpl, full); err != nil {
				t.Errorf("%s heading: %v", slug, err)
			}
		}
	}
}

func TestCatalog_Heading(t *testing.T) {
	t.Parallel()
	home := embedded(t).screens["home"]
	full := Fields{FirstName: "Thiago", Segment: "Advance", Since: "2024"}
	if got, err := execute(home.title, full); err != nil || got != "Olá, Thiago" {
		t.Errorf("title = %q, %v; want %q", got, err, "Olá, Thiago")
	}
	if got, err := execute(home.subtitle, full); err != nil || got != "Cliente Advance desde 2024" {
		t.Errorf("subtitle = %q, %v; want %q", got, err, "Cliente Advance desde 2024")
	}
	// Without advisory the engine renders no subtitle, and the title falls
	// back to the plain greeting.
	if got, err := execute(home.title, Fields{}); err != nil || got != "Olá" {
		t.Errorf("fallback title = %q, %v; want %q", got, err, "Olá")
	}
	if home.staticSubtitle {
		t.Error("home subtitle reads advisory fields but is marked static")
	}

	investir := embedded(t).screens["investir"]
	if got, err := execute(investir.title, Fields{}); err != nil || got != "Investir" {
		t.Errorf("investir title = %q, %v; want Investir", got, err)
	}
	if got, err := execute(investir.subtitle, Fields{}); err != nil || got != "Produtos fictícios · preço fixo da simulação" || !investir.staticSubtitle {
		t.Errorf("investir subtitle = %q, %v, static %t", got, err, investir.staticSubtitle)
	}
}

func TestCatalog_Text(t *testing.T) {
	t.Parallel()
	cat := embedded(t)
	got, err := cat.Text("advisor_card", "default", "meta", Fields{SLA: "4 h", Segment: "Advance"})
	if err != nil || got != "Resposta em até 4 h · cliente Advance" {
		t.Errorf("Text = %q, %v", got, err)
	}
	if _, err := cat.Text("advisor_card", "default", "missing", Fields{}); err == nil {
		t.Error("Text of a missing key returned no error")
	}
}

func TestCatalog_SLA(t *testing.T) {
	t.Parallel()
	cat := embedded(t)
	tests := []struct {
		segment  string
		expected string
	}{
		{segment: "Essencial", expected: "24 h"},
		{segment: "Advance", expected: "4 h"},
		{segment: "Singular", expected: "1 h"},
	}
	for _, tt := range tests {
		t.Run(strings.ToLower(tt.segment), func(t *testing.T) {
			t.Parallel()
			got, err := cat.SLA(tt.segment)
			if err != nil || got != tt.expected {
				t.Errorf("SLA(%q) = %q, %v; want %q", tt.segment, got, err, tt.expected)
			}
		})
	}
	if _, err := cat.SLA("Private"); err == nil {
		t.Error("SLA of an unknown segment returned no error")
	}
}

func TestParseCatalog_Invalid(t *testing.T) {
	t.Parallel()
	const section = `{"id":"moment","type":"moment_card","variants":["welcome"]}`
	screen := func(sections string) string {
		return `{"home":{"revision":"v1","revisions":{"v1":{"title":"Olá","subtitle":"s","sections":[` + sections + `]}}}}`
	}
	tests := []struct {
		name string
		raw  string
	}{
		{name: "malformed json", raw: `{`},
		{name: "unknown field", raw: `{"version":1,"screens":` + screen(section) + `,"extra":true}`},
		{name: "version zero", raw: `{"version":0,"screens":` + screen(section) + `}`},
		{name: "no screens", raw: `{"version":1,"screens":{}}`},
		{name: "served revision missing", raw: `{"version":1,"screens":{"home":{"revision":"v2","revisions":{"v1":{"title":"t","subtitle":"s","sections":[` + section + `]}}}}}`},
		{name: "no sections", raw: `{"version":1,"screens":` + screen(``) + `}`},
		{name: "section without variants", raw: `{"version":1,"screens":` + screen(`{"id":"moment","type":"moment_card","variants":[]}`) + `}`},
		{name: "repeated section", raw: `{"version":1,"screens":` + screen(section+`,`+section) + `}`},
		{name: "bad template", raw: `{"version":1,"screens":` + screen(section) + `,"copy":{"moment_card":{"welcome":{"title":"{{.FirstName"}}}}`},
		{name: "empty copy", raw: `{"version":1,"screens":` + screen(section) + `,"copy":{"moment_card":{"welcome":{"title":" "}}}}`},
		{name: "trailing object", raw: `{"version":1,"screens":` + screen(section) + `} {}`},
		{name: "trailing brace", raw: `{"version":1,"screens":` + screen(section) + `}}`},
		{name: "segment without sla", raw: `{"version":1,"screens":` + screen(section) + `,"segments":{"Advance":{"sla":""}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseCatalog([]byte(tt.raw)); !errors.Is(err, errCatalog) {
				t.Errorf("parseCatalog error = %v, want %v", err, errCatalog)
			}
		})
	}
}

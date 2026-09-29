package screen

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/template"
	"text/template/parse"
)

// catalogJSON is the screen catalog: per screen and revision the ordered
// sections, per variant its Portuguese copy templates, and per segment the
// SLA text. Keys are English. A change ships with a BFF rebuild (ADR 0009).
//
//go:embed catalog.json
var catalogJSON []byte

// Fields are the named values a copy template may use. Go formats every value
// before it lands here; a template only places text.
type Fields struct {
	FirstName   string
	AdvisorName string
	Segment     string
	Since       string
	SLA         string
	// Gap is the money missing to reach the next segment.
	Gap string
	// Threshold is the patrimony where Advance starts, as money.
	Threshold string
	// Cash is the cash balance as money.
	Cash string
	// CashShare is the percentage of patrimony held in cash.
	CashShare string
	// IdleDays is a day count such as "1 dia" or "4 dias".
	IdleDays string
	// Profile is the lowercase investor profile.
	Profile string
	// Protocol is the display protocol of a case.
	Protocol string
	// Age is a relative time such as "há 3 min".
	Age string
	// Risk is a product risk level, "1" to "5".
	Risk string
	// MaxRisk is the highest risk level the investor profile accepts.
	MaxRisk string
	// Minimum is a product's minimum purchase as compact money.
	Minimum string
}

// Catalog is the parsed, read-only screen catalog. It is safe for concurrent
// use.
type Catalog struct {
	version int
	screens map[string]screenDef
	copy    map[copyKey]*template.Template
	sla     map[string]string
}

// screenDef is the served revision of one screen. staticSubtitle is set when
// the subtitle template is plain text, so it needs no advisory field.
type screenDef struct {
	revision       string
	title          *template.Template
	subtitle       *template.Template
	staticSubtitle bool
	sections       []sectionDef
}

// sectionDef is one catalog section: its component type and its variants in
// evaluation order, the last being the default.
type sectionDef struct {
	id       string
	typ      string
	variants []string
}

type copyKey struct {
	typ     string
	variant string
	key     string
}

// catalogFile is the JSON layout of catalog.json.
type catalogFile struct {
	Version  int                                     `json:"version"`
	Screens  map[string]screenFile                   `json:"screens"`
	Copy     map[string]map[string]map[string]string `json:"copy"`
	Segments map[string]segmentFile                  `json:"segments"`
}

type screenFile struct {
	Revision  string                  `json:"revision"`
	Revisions map[string]revisionFile `json:"revisions"`
}

type revisionFile struct {
	Title    string        `json:"title"`
	Subtitle string        `json:"subtitle"`
	Sections []sectionFile `json:"sections"`
}

type sectionFile struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Variants []string `json:"variants"`
}

type segmentFile struct {
	SLA string `json:"sla"`
}

// errCatalog marks an invalid catalog file.
var errCatalog = errors.New("screen: invalid catalog")

// parseCatalog decodes and validates a catalog file and parses every
// template. Unknown JSON fields and data after the object are rejected so a
// misspelt key or a bad merge fails the build instead of silently dropping
// copy.
func parseCatalog(raw []byte) (Catalog, error) {
	var file catalogFile
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return Catalog{}, fmt.Errorf("%w: decode: %w", errCatalog, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Catalog{}, fmt.Errorf("%w: data after the catalog object", errCatalog)
	}
	if file.Version < 1 {
		return Catalog{}, fmt.Errorf("%w: version must be at least 1", errCatalog)
	}
	if len(file.Screens) == 0 {
		return Catalog{}, fmt.Errorf("%w: no screens", errCatalog)
	}
	cat := Catalog{
		version: file.Version,
		screens: make(map[string]screenDef, len(file.Screens)),
		copy:    map[copyKey]*template.Template{},
		sla:     make(map[string]string, len(file.Segments)),
	}
	for slug, sf := range file.Screens {
		def, err := parseScreen(slug, sf)
		if err != nil {
			return Catalog{}, err
		}
		cat.screens[slug] = def
	}
	for typ, variants := range file.Copy {
		for variant, texts := range variants {
			for key, text := range texts {
				name := typ + "/" + variant + "/" + key
				tmpl, err := parseTemplate(name, text)
				if err != nil {
					return Catalog{}, err
				}
				cat.copy[copyKey{typ: typ, variant: variant, key: key}] = tmpl
			}
		}
	}
	for segment, sf := range file.Segments {
		if strings.TrimSpace(sf.SLA) == "" {
			return Catalog{}, fmt.Errorf("%w: segment %q has no sla", errCatalog, segment)
		}
		cat.sla[segment] = sf.SLA
	}
	return cat, nil
}

// parseScreen validates every revision of a screen and keeps the served one.
func parseScreen(slug string, sf screenFile) (screenDef, error) {
	if _, ok := sf.Revisions[sf.Revision]; !ok {
		return screenDef{}, fmt.Errorf("%w: screen %q serves missing revision %q", errCatalog, slug, sf.Revision)
	}
	var served screenDef
	for rev, rf := range sf.Revisions {
		def, err := parseRevision(slug, rev, rf)
		if err != nil {
			return screenDef{}, err
		}
		if rev == sf.Revision {
			served = def
		}
	}
	return served, nil
}

func parseRevision(slug, rev string, rf revisionFile) (screenDef, error) {
	where := slug + "/" + rev
	if len(rf.Sections) == 0 {
		return screenDef{}, fmt.Errorf("%w: %s has no sections", errCatalog, where)
	}
	title, err := parseTemplate(where+"/title", rf.Title)
	if err != nil {
		return screenDef{}, err
	}
	subtitle, err := parseTemplate(where+"/subtitle", rf.Subtitle)
	if err != nil {
		return screenDef{}, err
	}
	def := screenDef{
		revision:       rev,
		title:          title,
		subtitle:       subtitle,
		staticSubtitle: isPlainText(subtitle),
		sections:       make([]sectionDef, 0, len(rf.Sections)),
	}
	seen := make(map[string]struct{}, len(rf.Sections))
	for _, s := range rf.Sections {
		if s.ID == "" || s.Type == "" || len(s.Variants) == 0 {
			return screenDef{}, fmt.Errorf("%w: %s has a section without id, type, or variants", errCatalog, where)
		}
		if _, dup := seen[s.ID]; dup {
			return screenDef{}, fmt.Errorf("%w: %s repeats section %q", errCatalog, where, s.ID)
		}
		seen[s.ID] = struct{}{}
		def.sections = append(def.sections, sectionDef{id: s.ID, typ: s.Type, variants: s.Variants})
	}
	return def, nil
}

// parseTemplate parses one copy template. Fields is a struct, so a template
// naming a field Fields lacks fails when executed, which the catalog test
// does for every template.
func parseTemplate(name, text string) (*template.Template, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("%w: %s is empty", errCatalog, name)
	}
	tmpl, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errCatalog, name, err)
	}
	return tmpl, nil
}

// isPlainText reports whether tmpl holds only text and no action, so it
// renders the same whatever the fields.
func isPlainText(tmpl *template.Template) bool {
	for _, node := range tmpl.Root.Nodes {
		if node.Type() != parse.NodeText {
			return false
		}
	}
	return true
}

// Version is the catalog file version.
func (c Catalog) Version() int { return c.version }

// Text executes the copy template key of a component type and variant.
func (c Catalog) Text(componentType, variant, key string, f Fields) (string, error) {
	tmpl, ok := c.copy[copyKey{typ: componentType, variant: variant, key: key}]
	if !ok {
		return "", fmt.Errorf("screen: no copy %s/%s/%s", componentType, variant, key)
	}
	return execute(tmpl, f)
}

// SLA is the advisory response-time text of a segment, such as "4 h".
func (c Catalog) SLA(segment string) (string, error) {
	sla, ok := c.sla[segment]
	if !ok {
		return "", fmt.Errorf("screen: no sla for segment %q", segment)
	}
	return sla, nil
}

func execute(tmpl *template.Template, f Fields) (string, error) {
	var b strings.Builder
	if err := tmpl.Execute(&b, f); err != nil {
		return "", fmt.Errorf("screen: execute %s: %w", tmpl.Name(), err)
	}
	return b.String(), nil
}

// copier reads several copy keys of one variant and keeps the first error,
// so a Build can read its copy in a row and check once.
type copier struct {
	cat     Catalog
	typ     string
	variant string
	fields  Fields
	err     error
}

func (cp *copier) text(key string) string {
	if cp.err != nil {
		return ""
	}
	s, err := cp.cat.Text(cp.typ, cp.variant, key, cp.fields)
	if err != nil {
		cp.err = err
		return ""
	}
	return s
}

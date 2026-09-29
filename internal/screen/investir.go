package screen

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Component types of the Investir screen.
const (
	typeInvestSummary = "invest_summary"
	typeProductRail   = "product_rail"
	typeProductList   = "product_list"
)

// copyProduct keys the catalog copy every product card shares. It is not a
// component type: product_rail and product_list both read it.
const copyProduct = "product"

// Asset classes of the account-sim catalog.
const (
	classAcoes     = "acoes"
	classETFs      = "etfs"
	classRendaFixa = "renda_fixa"
)

// Risk bounds of a catalog product; web draws one bar per level.
const (
	minRisk = 1
	maxRisk = 5
)

// highlightPicks is the deterministic product_rail pick per investor
// profile, in display order (architecture.md "Investor profiles"). The
// variant for a profile is "profile_" + the profile.
var highlightPicks = map[string][]string{
	"conservador": {"tbill", "corp"},
	"moderado":    {"acoesg", "corp"},
	"arrojado":    {"cobalto", "acoesg"},
}

// investirVariants registers the Investir variants and the sources each
// reads. The investor profile is optional for invest_summary and the lists:
// without it the chip and the above-profile badges are left out.
func investirVariants() map[variantKey]registered {
	catalog := []Source{SourceCatalog}
	variants := map[variantKey]registered{
		{typeInvestSummary, "default"}:    {variant: investSummary{}, needs: []Source{SourceAccount}},
		{typeProductList, "fixed_income"}: {variant: productList{name: "fixed_income", class: classRendaFixa}, needs: catalog},
		{typeProductList, "etf"}:          {variant: productList{name: "etf", class: classETFs}, needs: catalog},
		{typeProductList, "stocks"}:       {variant: productList{name: "stocks", class: classAcoes}, needs: catalog},
	}
	for profile, picks := range highlightPicks {
		name := "profile_" + profile
		variants[variantKey{typ: typeProductRail, name: name}] = registered{
			variant: productRail{name: name, profile: profile, picks: picks},
			needs:   []Source{SourceCatalog, SourceProfile},
		}
	}
	return variants
}

// investorProfile is the customer's profile when advisory answered with a
// known one: a name and a positive max risk. ok is false otherwise, and the
// screen then shows no chip and no above-profile badge.
func investorProfile(s Snapshot) (profile string, limit int, ok bool) {
	if !s.Profile.OK() {
		return "", 0, false
	}
	p := s.Profile.Value
	name := strings.ToLower(p.Profile)
	if name == "" || p.MaxRisk <= 0 {
		return "", 0, false
	}
	return name, p.MaxRisk, true
}

// investSummary is the cash available to invest and the profile chip.
type investSummary struct{}

func (investSummary) Matches(Snapshot) bool { return true }

func (investSummary) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Account.OK() {
		return Component{}, fmt.Errorf("screen: invest summary needs the account: %w", s.Account.Err)
	}
	cash := s.Account.Value.Cash
	props := InvestSummary{Cash: Money(cash), CashCents: cash}
	cp := copier{cat: c, typ: typeInvestSummary, variant: "default"}
	if profile, _, ok := investorProfile(s); ok {
		cp.fields.Profile = profile
		props.ProfileChip = cp.text("profile_chip")
	}
	props.CashLabel = cp.text("cash_label")
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeInvestSummary, Variant: "default", Props: props}, nil
}

// productRail is the highlights pick for one investor profile. It matches
// only that profile; an unknown profile matches no variant, and the section
// is dropped as a build error.
type productRail struct {
	name    string
	profile string
	picks   []string
}

func (v productRail) Matches(s Snapshot) bool {
	profile, _, ok := investorProfile(s)
	return ok && profile == v.profile
}

func (v productRail) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Products.OK() {
		return Component{}, fmt.Errorf("screen: highlights need the catalog: %w", s.Products.Err)
	}
	profile, _, ok := investorProfile(s)
	if !ok {
		return Component{}, errors.New("screen: highlights need the investor profile")
	}
	items := make([]ProductItem, 0, len(v.picks))
	for _, id := range v.picks {
		i := slices.IndexFunc(s.Products.Value, func(p Product) bool { return p.ID == id })
		if i < 0 {
			return Component{}, fmt.Errorf("screen: highlight %q is not in the catalog", id)
		}
		item, err := productItem(s, c, s.Products.Value[i])
		if err != nil {
			return Component{}, err
		}
		items = append(items, item)
	}
	cp := copier{cat: c, typ: typeProductRail, variant: v.name, fields: Fields{Profile: profile}}
	props := ProductRail{Title: cp.text("title"), Subtitle: cp.text("subtitle"), Products: items}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeProductRail, Variant: v.name, Props: props}, nil
}

// productList is every catalog product of one asset class, by risk and then
// id. A class with no product is a build error.
type productList struct {
	name  string
	class string
}

func (productList) Matches(Snapshot) bool { return true }

func (v productList) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Products.OK() {
		return Component{}, fmt.Errorf("screen: product list needs the catalog: %w", s.Products.Err)
	}
	products := make([]Product, 0, len(s.Products.Value))
	for _, p := range s.Products.Value {
		if p.AssetClass == v.class {
			products = append(products, p)
		}
	}
	if len(products) == 0 {
		return Component{}, fmt.Errorf("screen: the catalog has no %s product", v.class)
	}
	slices.SortFunc(products, func(a, b Product) int {
		return cmp.Or(cmp.Compare(a.Risk, b.Risk), cmp.Compare(a.ID, b.ID))
	})
	items := make([]ProductItem, 0, len(products))
	for _, p := range products {
		item, err := productItem(s, c, p)
		if err != nil {
			return Component{}, err
		}
		items = append(items, item)
	}
	cp := copier{cat: c, typ: typeProductList, variant: v.name}
	props := ProductList{Title: cp.text("title"), Products: items}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeProductList, Variant: v.name, Props: props}, nil
}

// productItem is one product card. A product is above profile when its risk
// exceeds the profile's max risk, the same test advisory's suitability rule
// applies; without a known profile no product is above it.
func productItem(s Snapshot, c Catalog, p Product) (ProductItem, error) {
	if p.ID == "" || p.Name == "" {
		return ProductItem{}, errors.New("screen: catalog product without id or name")
	}
	if p.Risk < minRisk || p.Risk > maxRisk {
		return ProductItem{}, fmt.Errorf("screen: product %q risk %d is outside %d–%d", p.ID, p.Risk, minRisk, maxRisk)
	}
	f := Fields{Risk: strconv.Itoa(p.Risk), Minimum: CompactMoney(p.MinimumCents)}
	profile, limit, known := investorProfile(s)
	above := known && p.Risk > limit
	if known {
		f.Profile = profile
		f.MaxRisk = strconv.Itoa(limit)
	}
	cp := copier{cat: c, typ: copyProduct, variant: "default", fields: f}
	item := ProductItem{
		ProductID:    p.ID,
		Name:         p.Name,
		ClassLabel:   cp.text("class_" + p.AssetClass),
		Risk:         p.Risk,
		RiskLabel:    cp.text("risk_label"),
		ReturnLabel:  p.ReturnLabel,
		Minimum:      cp.text("minimum"),
		MinimumCents: p.MinimumCents,
		AboveProfile: above,
		Action:       Action{Type: ActionPanel, Label: cp.text("action"), Target: PanelPurchase, ProductID: p.ID},
	}
	if above {
		item.Badge = cp.text("badge")
		item.Warning = cp.text("warning")
	}
	if cp.err != nil {
		return ProductItem{}, cp.err
	}
	return item, nil
}

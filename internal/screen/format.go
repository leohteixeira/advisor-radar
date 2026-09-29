package screen

import (
	"errors"
	"math/big"
	"math/bits"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// minus is U+2212, the sign every negative display value uses.
const minus = "−"

// errShareOverflow is returned by Shares when the values sum past uint64.
var errShareOverflow = errors.New("screen: shares overflow")

// Money formats integer USD cents as "US$ 1.234,56" with pt-BR grouping. A
// negative amount reads "− US$ 1.234,56" with U+2212.
func Money(cents int64) string {
	if cents < 0 {
		return minus + " " + unsignedMoney(magnitude(cents))
	}
	return unsignedMoney(magnitude(cents))
}

// SignedMoney formats integer USD cents with an explicit sign and no space
// after it, as a return or a movement reads: "+US$ 19.400,00" and
// "−US$ 250,00" with U+2212. Zero has no sign: "US$ 0,00".
func SignedMoney(cents int64) string {
	switch {
	case cents > 0:
		return "+" + unsignedMoney(magnitude(cents))
	case cents < 0:
		return minus + unsignedMoney(magnitude(cents))
	default:
		return unsignedMoney(0)
	}
}

// magnitude is |n| as a uint64, exact even for math.MinInt64.
func magnitude(n int64) uint64 {
	m := uint64(n)
	if n < 0 {
		m = -m // two's complement
	}
	return m
}

// unsignedMoney writes cents as "US$ 1.234,56".
func unsignedMoney(cents uint64) string {
	var b strings.Builder
	b.WriteString("US$ ")
	b.WriteString(groupThousands(cents / 100))
	b.WriteByte(',')
	frac := cents % 100
	if frac < 10 {
		b.WriteByte('0')
	}
	b.WriteString(strconv.FormatUint(frac, 10))
	return b.String()
}

// CompactMoney is Money without the cents of a whole-dollar amount, as a
// product minimum reads: "US$ 1.000", but "US$ 12,50".
func CompactMoney(cents int64) string {
	if cents%100 != 0 {
		return Money(cents)
	}
	return strings.TrimSuffix(Money(cents), ",00")
}

// groupThousands writes n with "." between groups of three digits.
func groupThousands(n uint64) string {
	return groupDigits(strconv.FormatUint(n, 10))
}

// groupDigits writes a string of decimal digits with "." between groups of
// three.
func groupDigits(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	b.Grow(len(digits) + len(digits)/3)
	lead := len(digits) % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(digits[:lead])
	for i := lead; i < len(digits); i += 3 {
		b.WriteByte('.')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// Percent formats a whole percentage as "62%".
func Percent(p int) string {
	return strconv.Itoa(p) + "%"
}

// ChangePercent formats delta as a signed percentage of base with one
// decimal, rounded half away from zero: "+11,5%", "−53,5%" with U+2212. A
// zero delta, or a base that is not positive (there is nothing to measure
// against), reads "0,0%". The sign always follows delta, so a gain too small
// to show reads "+0,0%".
func ChangePercent(delta, base int64) string {
	if delta == 0 || base <= 0 {
		return "0,0%"
	}
	// tenths = round(|delta| · 1000 / base), exact at any magnitude.
	num := new(big.Int).Mul(new(big.Int).SetUint64(magnitude(delta)), big.NewInt(1000))
	den := big.NewInt(base)
	tenths, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Lsh(rem, 1).Cmp(den) >= 0 {
		tenths.Add(tenths, big.NewInt(1))
	}
	whole, frac := new(big.Int).QuoRem(tenths, big.NewInt(10), new(big.Int))
	sign := "+"
	if delta < 0 {
		sign = minus
	}
	return sign + groupDigits(whole.String()) + "," + frac.String() + "%"
}

// SignTone is the tone of a signed amount: TonePos above zero, ToneNeg below,
// and ToneNeutral at zero.
func SignTone(n int64) string {
	switch {
	case n > 0:
		return TonePos
	case n < 0:
		return ToneNeg
	default:
		return ToneNeutral
	}
}

// Shares splits 100 percentage points across values by largest remainder, so
// the shares sum to exactly 100 whenever the total is positive. Each value
// gets the floor of its exact share; the points left go one each to the
// largest remainders, ties to the earlier value. Negative values count as
// zero. All shares are zero when the total is zero.
func Shares(values []int64) ([]int, error) {
	shares := make([]int, len(values))
	var total uint64
	for _, v := range values {
		var carry uint64
		total, carry = bits.Add64(total, nonNegative(v), 0)
		if carry != 0 {
			return nil, errShareOverflow
		}
	}
	if total == 0 {
		return shares, nil
	}
	remainders := make([]uint64, len(values))
	assigned := 0
	for i, v := range values {
		// v·100 in 128 bits; v ≤ total keeps the high word below total, which
		// Div64 requires.
		hi, lo := bits.Mul64(nonNegative(v), 100)
		q, r := bits.Div64(hi, lo, total)
		shares[i] = int(q) // q ≤ 100
		remainders[i] = r
		assigned += int(q)
	}
	order := make([]int, len(values))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		switch {
		case remainders[a] > remainders[b]:
			return -1
		case remainders[a] < remainders[b]:
			return 1
		default:
			return 0
		}
	})
	for _, i := range order[:100-assigned] {
		shares[i]++
	}
	return shares, nil
}

func nonNegative(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

// RelativeTime formats how long ago something happened: "agora" under a
// minute, "há 5 min", "há 3 h", "ontem" from 24 to 48 hours, then "há 4 dias".
// A negative age reads "agora".
func RelativeTime(age time.Duration) string {
	day := 24 * time.Hour
	switch {
	case age < time.Minute:
		return "agora"
	case age < time.Hour:
		return "há " + strconv.FormatInt(int64(age/time.Minute), 10) + " min"
	case age < day:
		return "há " + strconv.FormatInt(int64(age/time.Hour), 10) + " h"
	case age < 2*day:
		return "ontem"
	default:
		return "há " + strconv.FormatInt(int64(age/day), 10) + " dias"
	}
}

// Days formats a whole day count: "1 dia", else "4 dias".
func Days(n int) string {
	if n == 1 {
		return "1 dia"
	}
	return strconv.Itoa(n) + " dias"
}

// protocolLength is how many leading characters of a case id make its
// display protocol: the first two UUID groups and their hyphen.
const protocolLength = 13

// Protocol is the display protocol of a case: the first two groups of its
// id, uppercased, as in "01A0E3A5-2F4C". A shorter id is used whole.
func Protocol(id string) string {
	if len(id) > protocolLength {
		id = id[:protocolLength]
	}
	return strings.ToUpper(id)
}

// FirstName is the first word of a display name.
func FirstName(name string) string {
	for word := range strings.FieldsSeq(name) {
		return word
	}
	return ""
}

// nameConnectors are the Portuguese particles Initials skips.
var nameConnectors = map[string]struct{}{
	"da": {}, "das": {}, "de": {}, "do": {}, "dos": {}, "e": {},
}

// Initials are the upper-cased first letters of the first two words of a
// name, skipping Portuguese connectors, so "Ana Paula Ribeiro" is "AP" and
// "Maria da Silva" is "MS".
func Initials(name string) string {
	var b strings.Builder
	count := 0
	for word := range strings.FieldsSeq(name) {
		if _, skip := nameConnectors[strings.ToLower(word)]; skip {
			continue
		}
		r, _ := utf8.DecodeRuneInString(word)
		b.WriteRune(unicode.ToUpper(r))
		count++
		if count == 2 {
			break
		}
	}
	return b.String()
}

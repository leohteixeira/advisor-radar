package sim_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// seedSQL returns every seeds/account_sim/*.sql file joined in apply order.
func seedSQL(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "seeds", "account_sim")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read seeds: %v", err)
	}
	var b strings.Builder
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		b.Write(body)
		b.WriteByte('\n')
	}
	return b.String()
}

// seedRows parses the VALUES tuples of the single INSERT into table. The seed
// files keep one tuple per line of quoted strings and integers. The block ends
// at an ON CONFLICT line or at a tuple ending in ";"; blank and comment lines
// are skipped, and any other line fails the test.
func seedRows(t *testing.T, sql, table string) [][]string {
	t.Helper()
	head := "INSERT INTO " + table + " ("
	start := strings.Index(sql, head)
	if start < 0 {
		t.Fatalf("no INSERT INTO %s in the seed", table)
	}
	if strings.Contains(sql[start+len(head):], head) {
		t.Fatalf("more than one INSERT INTO %s in the seed", table)
	}
	lines := strings.Split(sql[start:], "\n")
	if !strings.HasSuffix(strings.TrimSpace(lines[0]), "VALUES") {
		t.Fatalf("INSERT INTO %s: header %q does not end in VALUES", table, lines[0])
	}
	var rows [][]string
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "--"):
			continue
		case strings.HasPrefix(line, "ON CONFLICT"):
			return rows
		case strings.HasPrefix(line, "("):
			if tuple, ok := strings.CutSuffix(line, ";"); ok {
				return append(rows, parseTuple(t, tuple))
			}
			rows = append(rows, parseTuple(t, strings.TrimSuffix(line, ",")))
		default:
			t.Fatalf("INSERT INTO %s: unexpected line %q in VALUES", table, line)
		}
	}
	t.Fatalf("INSERT INTO %s: VALUES block has no end", table)
	return nil
}

func parseTuple(t *testing.T, tuple string) []string {
	t.Helper()
	if !strings.HasPrefix(tuple, "(") || !strings.HasSuffix(tuple, ")") {
		t.Fatalf("tuple %q is not parenthesized", tuple)
	}
	body := tuple[1 : len(tuple)-1]
	var fields []string
	for i := 0; i < len(body); {
		switch {
		case body[i] == ' ' || body[i] == ',':
			i++
		case body[i] == '\'':
			var field strings.Builder
			i++
			for {
				if i >= len(body) {
					t.Fatalf("unterminated string in %q", tuple)
				}
				if body[i] == '\'' {
					if i+1 < len(body) && body[i+1] == '\'' {
						field.WriteByte('\'')
						i += 2
						continue
					}
					i++
					break
				}
				field.WriteByte(body[i])
				i++
			}
			fields = append(fields, field.String())
		default:
			end := strings.IndexByte(body[i:], ',')
			if end < 0 {
				end = len(body) - i
			}
			fields = append(fields, strings.TrimSpace(body[i:i+end]))
			i += end
		}
	}
	return fields
}

func atoi64(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return n
}

// TestSeedSQL_MatchesGoSeed proves the SQL seed that `cmd/db seed` applies
// and DemoSeed, which Reseed and Memory use, hold the same rows.
func TestSeedSQL_MatchesGoSeed(t *testing.T) {
	t.Parallel()
	sql := seedSQL(t)
	seed := sim.DemoSeed()

	var products []sim.Product
	for _, row := range seedRows(t, sql, "pov_product") {
		if len(row) != 6 {
			t.Fatalf("pov_product row %v has %d fields", row, len(row))
		}
		products = append(products, sim.Product{
			ID: row[0], Name: row[1], AssetClass: row[2], Risk: int(atoi64(t, row[3])),
			ReturnLabel: row[4], MinimumCents: atoi64(t, row[5]),
		})
	}
	if !slices.Equal(products, seed.Products) {
		t.Fatalf("SQL products = %+v, Go = %+v", products, seed.Products)
	}

	type position struct {
		customerID, productID string
		units, applied        int64
	}
	var sqlPositions []position
	for _, row := range seedRows(t, sql, "pov_position") {
		if len(row) != 4 {
			t.Fatalf("pov_position row %v has %d fields", row, len(row))
		}
		sqlPositions = append(sqlPositions, position{row[0], row[1], atoi64(t, row[2]), atoi64(t, row[3])})
	}
	cash := map[string]int64{}
	for _, row := range seedRows(t, sql, "pov_account") {
		if len(row) != 2 {
			t.Fatalf("pov_account row %v has %d fields", row, len(row))
		}
		cash[row[0]] = atoi64(t, row[1])
	}
	registrations := map[string]sim.Registration{}
	for _, row := range seedRows(t, sql, "pov_registration") {
		if len(row) != 5 {
			t.Fatalf("pov_registration row %v has %d fields", row, len(row))
		}
		registrations[row[0]] = sim.Registration{
			CustomerID: row[0], Email: row[1], Phone: row[2], City: row[3], AccountNumber: row[4],
		}
	}

	preferences := map[string]sim.Preferences{}
	for _, row := range seedRows(t, sql, "pov_preferences") {
		if len(row) != 3 {
			t.Fatalf("pov_preferences row %v has %d fields", row, len(row))
		}
		beta, err := strconv.ParseBool(row[2])
		if err != nil {
			t.Fatalf("pov_preferences beta %q: %v", row[2], err)
		}
		preferences[row[0]] = sim.Preferences{Channel: row[1], Beta: beta}
	}

	var goPositions []position
	for _, account := range seed.Accounts {
		if got, ok := preferences[account.CustomerID]; !ok || got != account.Preferences {
			t.Fatalf("%s SQL preferences = %+v (present %t), Go = %+v", account.CustomerID, got, ok, account.Preferences)
		}
		got, ok := cash[account.CustomerID]
		if !ok || got != account.CashCents {
			t.Fatalf("%s SQL cash = %d (present %t), Go = %d", account.CustomerID, got, ok, account.CashCents)
		}
		if registrations[account.CustomerID] != account.Registration {
			t.Fatalf("%s SQL registration = %+v, Go = %+v",
				account.CustomerID, registrations[account.CustomerID], account.Registration)
		}
		for _, p := range account.Positions {
			goPositions = append(goPositions, position{account.CustomerID, p.ProductID, p.UnitsCents, p.AppliedCents})
		}
	}
	if len(cash) != len(seed.Accounts) || len(registrations) != len(seed.Accounts) || len(preferences) != len(seed.Accounts) {
		t.Fatalf("SQL has %d accounts, %d registrations, and %d preferences, Go has %d",
			len(cash), len(registrations), len(preferences), len(seed.Accounts))
	}
	if !slices.Equal(sqlPositions, goPositions) {
		t.Fatalf("SQL positions = %+v, Go = %+v", sqlPositions, goPositions)
	}
}

package seeds_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/identity"
)

var uuidRE = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func TestSeedSQL_UUIDv7AndCrossFileIDs(t *testing.T) {
	t.Parallel()
	root := seedsRoot(t)
	files := map[string]string{}
	for _, service := range []string{"advisory", "account_sim", "triage", "cases"} {
		dir := filepath.Join(root, service)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			files[filepath.Join(service, e.Name())] = string(body)
		}
	}
	if len(files) == 0 {
		t.Fatal("no seed sql files")
	}

	allIDs := map[string]struct{}{}
	for name, body := range files {
		for _, m := range uuidRE.FindAllString(body, -1) {
			if !identity.IsV7(m) {
				t.Fatalf("%s: %q is not uuidv7", name, m)
			}
			allIDs[strings.ToLower(m)] = struct{}{}
		}
	}

	advisory := files["advisory/001_cast.sql"]
	account := files["account_sim/001_cast.sql"]
	triage := files["triage/001_cast.sql"]
	casesSQL := files["cases/001_cast.sql"]

	// Mariana (c01), Ana operator, and shared signal/case/event ids must appear
	// wherever the cast references them.
	mustContain := []struct {
		label string
		id    string
		in    []string
	}{
		{"mariana", "01a0e3a4-9a44-7566-b5de-eb2e365799f8", []string{advisory, account, triage, casesSQL}},
		{"ana", "01a0e3a4-9a44-750d-9e38-ca5f92ecf52a", []string{advisory, casesSQL}},
		{"s01", "01a0e3a4-9a44-7640-9d53-0fb421112eed", []string{advisory}},
		{"k1038", "01a0e3a4-9a44-76f2-9263-f892becdd32a", []string{casesSQL}},
		{"tl-nota", "01a0e3a4-9a44-7763-bae8-4b24a2f3f3f2", []string{advisory}},
		{"ev-msg-s01", "01a0e3a4-9a44-7831-8b59-09e3634d198d", []string{account, triage}},
	}
	for _, tc := range mustContain {
		for _, body := range tc.in {
			if !strings.Contains(strings.ToLower(body), tc.id) {
				t.Fatalf("%s id %s missing from a required seed file", tc.label, tc.id)
			}
		}
	}

	// The POV accounts (cash) and their positions, registration (ADR 0010),
	// and preferences use the same customer ids as the advisory book.
	povAccounts := files["account_sim/002_pov_accounts.sql"]
	povPositions := files["account_sim/003_pov_positions.sql"]
	povPreferences := files["account_sim/004_pov_preferences.sql"]
	for _, pov := range []struct{ label, id string }{
		{"mariana", "01a0e3a4-9a44-7566-b5de-eb2e365799f8"},
		{"fernanda", "01a0e3a4-9a44-757a-ac8f-dab7db5eb068"},
		{"thiago", "01a0e3a4-9a44-75dd-b3a0-403a7a87836e"},
	} {
		for _, body := range []string{advisory, povAccounts, povPositions, povPreferences} {
			if !strings.Contains(strings.ToLower(body), pov.id) {
				t.Fatalf("pov customer %s id %s missing from a required seed file", pov.label, pov.id)
			}
		}
	}

	if len(allIDs) < 50 {
		t.Fatalf("too few distinct ids: %d", len(allIDs))
	}
}

func seedsRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "seeds")
}

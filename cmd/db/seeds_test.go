package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	seedUUID    = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}`)
	seedMinutes = regexp.MustCompile(`now\(\) - interval '(\d+) minutes'`)
	seedText    = regexp.MustCompile(`"text":"((?:[^"\\]|\\.)*)"`)
	seedCaseID  = regexp.MustCompile(`'case_id', '([0-9a-f-]{36})'`)
	seedKind    = regexp.MustCompile(`', '(message|alert)', now\(\)`)
)

// seedInsert is one INSERT of a seed file: its table, text, the UUIDs it
// names in order, and the age in minutes it gives each clock.
type seedInsert struct {
	table   string
	sql     string
	ids     []string
	minutes []int
}

// age is the single age of the insert. Every insert the checks read stamps
// one clock; one that stamps none or several fails the test.
func (s seedInsert) age(t *testing.T) int {
	t.Helper()
	if len(s.minutes) != 1 {
		t.Fatalf("%s insert stamps %d clocks, want 1: %.120s", s.table, len(s.minutes), s.sql)
	}
	return s.minutes[0]
}

// seedInserts parses the INSERT statements of every seed file of a service.
// The seed files end each statement with ";" at the end of a line.
func seedInserts(t *testing.T, service string) []seedInsert {
	t.Helper()
	dir := filepath.Join("..", "..", "seeds", service)
	files, err := listSQL(dir)
	if err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	var inserts []seedInsert
	for _, name := range files {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for stmt := range strings.SplitSeq(string(body), ";\n") {
			var lines []string
			for line := range strings.SplitSeq(stmt, "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "--") {
					lines = append(lines, line)
				}
			}
			sql := strings.TrimSpace(strings.Join(lines, "\n"))
			table, ok := strings.CutPrefix(sql, "INSERT INTO ")
			if !ok {
				continue
			}
			table, _, _ = strings.Cut(table, " ")
			insert := seedInsert{table: table, sql: sql, ids: seedUUID.FindAllString(sql, -1)}
			for _, match := range seedMinutes.FindAllStringSubmatch(sql, -1) {
				minutes, err := strconv.Atoi(match[1])
				if err != nil {
					t.Fatalf("minutes %q: %v", match[1], err)
				}
				insert.minutes = append(insert.minutes, minutes)
			}
			inserts = append(inserts, insert)
		}
	}
	if len(inserts) == 0 {
		t.Fatalf("no INSERT in seeds/%s", service)
	}
	return inserts
}

// text is the message text of an insert's JSON payload.
func (s seedInsert) text(t *testing.T) string {
	t.Helper()
	match := seedText.FindStringSubmatch(s.sql)
	if match == nil {
		t.Fatalf("%s insert has no text: %.120s", s.table, s.sql)
	}
	return match[1]
}

// One fact is seeded once in account-sim and has one age, whichever
// service's seed states it. The queue, the
// review, the timeline, and the cases each read their own service's copy, so
// two offsets for the same message show it with two ages on two screens.
//
//   - A queue signal and a triage result carry the offset of their message.
//   - A message with a seeded queue signal is claimed in the triage inbox, so
//     it is not classified again into a second card.
//   - An alert, its queue signal, and its event carry one offset, never older
//     than the account event that raised it.
//   - A case opens at the offset of its signal, as its case.opened event and
//     its first history line say, and no history line is older than that.
func TestSeeds_OneFactOneAge(t *testing.T) {
	t.Parallel()

	type message struct{ customer, text string }
	messageByID := map[string]int{}
	messageByText := map[message]int{}
	messageIDByText := map[message]string{}
	accountEvents := map[string]int{}
	for _, insert := range seedInserts(t, "account_sim") {
		if insert.table != "outbox" {
			continue
		}
		// event_id, event_id again in the body, customer_id.
		if len(insert.ids) < 3 {
			t.Fatalf("outbox insert names %d ids: %.120s", len(insert.ids), insert.sql)
		}
		switch {
		case strings.Contains(insert.sql, "'message.received'"):
			age := insert.age(t)
			key := message{customer: insert.ids[2], text: insert.text(t)}
			// The same message under two event ids is listed twice wherever
			// messages are listed.
			if _, ok := messageByText[key]; ok {
				t.Errorf("message %q of %s is seeded twice", key.text, key.customer)
			}
			messageByID[insert.ids[0]] = age
			messageByText[key] = age
			messageIDByText[key] = insert.ids[0]
		case strings.Contains(insert.sql, "'account.event.recorded'"):
			accountEvents[insert.ids[0]] = insert.age(t)
		}
	}
	if len(messageByID) < 10 || len(accountEvents) < 5 {
		t.Fatalf("parsed %d messages and %d account events from seeds/account_sim", len(messageByID), len(accountEvents))
	}

	// The messages triage will not classify at start: its seed claims them.
	claimed := map[string]bool{}
	for _, insert := range seedInserts(t, "triage") {
		if insert.table == "inbox" {
			for _, id := range insert.ids {
				claimed[id] = true
			}
		}
	}

	signals := map[string]int{}
	alerts := map[string]int{}
	alertEvents := map[string]int{}
	messageSignals := 0
	for _, insert := range seedInserts(t, "advisory") {
		switch {
		case insert.table == "queue_signals":
			age := insert.age(t)
			signals[insert.ids[0]] = age
			kind := seedKind.FindStringSubmatch(insert.sql)
			if kind == nil {
				t.Fatalf("queue signal %s has no kind", insert.ids[0])
			}
			if kind[1] != "message" {
				continue
			}
			messageSignals++
			key := message{customer: insert.ids[1], text: insert.text(t)}
			want, ok := messageByText[key]
			if !ok {
				t.Errorf("queue signal %s: no message %q of %s in seeds/account_sim", insert.ids[0], key.text, key.customer)
			} else if age != want {
				t.Errorf("queue signal %s is %d minutes old, its message %d", insert.ids[0], age, want)
			}
			// An unclaimed message is classified at start and reaches the
			// queue as a live card, next to this seeded one.
			if id := messageIDByText[key]; ok && !claimed[id] {
				t.Errorf("queue signal %s: seeds/triage does not claim its message %s, so the queue lists it twice", insert.ids[0], id)
			}
		case insert.table == "alerts":
			// id, customer_id, source_event_id.
			age := insert.age(t)
			alerts[insert.ids[0]] = age
			if event, ok := accountEvents[insert.ids[2]]; ok && age > event {
				t.Errorf("alert %s is %d minutes old, older than the account event that raised it (%d)", insert.ids[0], age, event)
			}
		case insert.table == "outbox" && strings.Contains(insert.sql, "'alert.raised'"):
			alertEvents[insert.ids[0]] = insert.age(t)
		}
	}
	if messageSignals < 10 || len(alerts) < 5 {
		t.Fatalf("parsed %d message signals and %d alerts from seeds/advisory", messageSignals, len(alerts))
	}
	for id, age := range alerts {
		if signal, ok := signals[id]; !ok || signal != age {
			t.Errorf("alert %s is %d minutes old, its queue signal %d (seeded: %v)", id, age, signal, ok)
		}
		if event, ok := alertEvents[id]; !ok || event != age {
			t.Errorf("alert %s is %d minutes old, its alert.raised event %d (seeded: %v)", id, age, event, ok)
		}
	}

	results := 0
	for _, insert := range seedInserts(t, "triage") {
		if insert.table != "results" {
			continue
		}
		// id, source_event_id, customer_id.
		results++
		age := insert.age(t)
		want, ok := messageByID[insert.ids[1]]
		if !ok {
			t.Errorf("triage result %s: no message %s in seeds/account_sim", insert.ids[0], insert.ids[1])
		} else if age != want {
			t.Errorf("triage result %s is %d minutes old, its message %d", insert.ids[0], age, want)
		}
	}
	if results < 8 {
		t.Fatalf("parsed %d results from seeds/triage", results)
	}

	opened := map[string]int{}
	firstLine := map[string]int{}
	openedEvents := map[string]int{}
	for _, insert := range seedInserts(t, "cases") {
		switch {
		case insert.table == "cases":
			// id, customer_id, signal_id, advisor_id.
			age := insert.age(t)
			opened[insert.ids[0]] = age
			if signal, ok := signals[insert.ids[2]]; !ok || signal != age {
				t.Errorf("case %s opened %d minutes ago, its signal was raised %d minutes ago (seeded: %v)", insert.ids[0], age, signal, ok)
			}
		case insert.table == "case_history":
			// id, case_id. The history follows the cases in the file.
			age := insert.age(t)
			caseID := insert.ids[1]
			if age > opened[caseID] {
				t.Errorf("history line %s of case %s is %d minutes old, older than the case (%d)", insert.ids[0], caseID, age, opened[caseID])
			}
			firstLine[caseID] = max(firstLine[caseID], age)
		case insert.table == "outbox" && strings.Contains(insert.sql, "'case.opened'"):
			match := seedCaseID.FindStringSubmatch(insert.sql)
			if match == nil {
				t.Fatalf("case.opened %s names no case", insert.ids[0])
			}
			openedEvents[match[1]] = insert.age(t)
		}
	}
	if len(opened) != 3 {
		t.Fatalf("parsed %d cases from seeds/cases, want the 3 of the cast", len(opened))
	}
	for id, age := range opened {
		if firstLine[id] != age {
			t.Errorf("case %s opened %d minutes ago, its first history line is %d minutes old", id, age, firstLine[id])
		}
		if event, ok := openedEvents[id]; !ok || event != age {
			t.Errorf("case %s opened %d minutes ago, its case.opened event says %d (seeded: %v)", id, age, event, ok)
		}
	}
}

package sim_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/sim"
)

func TestMessagePayloadFields(t *testing.T) {
	t.Parallel()
	p := sim.MessagePayload{Channel: "chat", Text: "hello"}
	if p.Channel != "chat" || p.Text != "hello" {
		t.Fatalf("unexpected %+v", p)
	}
}

func TestAccountPayload_Dollars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		payload       sim.AccountPayload
		schemaVersion int
		want          sim.AccountPayload
		wantErr       string
	}{
		{
			name: "version 1 dollars unchanged",
			payload: sim.AccountPayload{
				Kind:   "withdrawal",
				Amount: 60000,
				Before: 8000,
				After:  68000,
			},
			schemaVersion: 1,
			want: sim.AccountPayload{
				Kind:   "withdrawal",
				Amount: 60000,
				Before: 8000,
				After:  68000,
			},
		},
		{
			name: "version 2 cents to dollars",
			payload: sim.AccountPayload{
				Kind:        "aporte",
				Amount:      1_000_000,
				Before:      820_000,
				After:       1_820_000,
				Origin:      "pix",
				Destination: "conta-corrente",
			},
			schemaVersion: 2,
			want: sim.AccountPayload{
				Kind:        "aporte",
				Amount:      10_000,
				Before:      8_200,
				After:       18_200,
				Origin:      "pix",
				Destination: "conta-corrente",
			},
		},
		{
			name: "version 2 fractional cents",
			payload: sim.AccountPayload{
				Kind:   "deposit",
				Amount: 10000.5,
				Before: 820_000,
				After:  830_000,
			},
			schemaVersion: 2,
			wantErr:       "cents",
		},
		{
			name: "unsupported schema version",
			payload: sim.AccountPayload{
				Kind:   "deposit",
				Amount: 100,
				Before: 100,
				After:  200,
			},
			schemaVersion: 3,
			wantErr:       "schema_version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.payload.Dollars(tt.schemaVersion)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("Dollars() error = nil, want error")
				}
				if !errors.Is(err, sim.ErrMoneyScale) {
					t.Fatalf("Dollars() error = %v, want wrap of ErrMoneyScale", err)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Dollars() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Dollars() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Dollars() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

package sim_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// povCustomers are the three seeded POV clients.
var povCustomers = []string{sim.CustomerMariana, sim.CustomerFernanda, sim.CustomerThiago}

func preferencesOf(t *testing.T, client accountv1.AccountServiceClient, customerID string) sim.Preferences {
	t.Helper()
	got, err := client.GetPreferences(t.Context(), &accountv1.GetPreferencesRequest{CustomerId: customerID})
	if err != nil {
		t.Fatalf("GetPreferences(%s): %v", customerID, err)
	}
	return sim.Preferences{Channel: got.GetChannel(), Beta: got.GetBeta()}
}

// assertSeedPreferences checks that every POV client is on chat with beta off.
func assertSeedPreferences(t *testing.T, client accountv1.AccountServiceClient) {
	t.Helper()
	for _, id := range povCustomers {
		if got := preferencesOf(t, client, id); got != sim.DefaultPreferences() {
			t.Fatalf("%s preferences = %+v, want chat and beta off", id, got)
		}
	}
}

// assertPreferences runs the preferences matrix against a seeded store: set
// the channel, turn beta on, the refusals, and reseed. events counts the
// outbox rows, which no preference change may add; reseed restores the seed.
func assertPreferences(t *testing.T, client accountv1.AccountServiceClient, events func() int, reseed func()) {
	t.Helper()
	ctx := t.Context()
	assertSeedPreferences(t, client)
	before := events()

	update := func(channel string, beta bool) (*accountv1.Preferences, error) {
		return client.UpdatePreferences(ctx, &accountv1.UpdatePreferencesRequest{
			CustomerId: sim.CustomerFernanda, Channel: channel, Beta: beta,
		})
	}
	got, err := update(sim.ChannelEmail, false)
	if err != nil {
		t.Fatalf("UpdatePreferences(email): %v", err)
	}
	if got.GetChannel() != sim.ChannelEmail || got.GetBeta() {
		t.Fatalf("UpdatePreferences(email) = %+v", got)
	}
	if stored := preferencesOf(t, client, sim.CustomerFernanda); stored != (sim.Preferences{Channel: sim.ChannelEmail}) {
		t.Fatalf("stored after email = %+v", stored)
	}

	if got, err = update(sim.ChannelChat, true); err != nil {
		t.Fatalf("UpdatePreferences(beta): %v", err)
	}
	if got.GetChannel() != sim.ChannelChat || !got.GetBeta() {
		t.Fatalf("UpdatePreferences(beta) = %+v", got)
	}
	// The other clients keep their own rows.
	if other := preferencesOf(t, client, sim.CustomerThiago); other != sim.DefaultPreferences() {
		t.Fatalf("thiago preferences = %+v after fernanda's change", other)
	}

	for _, tc := range []struct {
		name     string
		customer string
		channel  string
		code     codes.Code
	}{
		{name: "sms channel", customer: sim.CustomerFernanda, channel: "sms", code: codes.InvalidArgument},
		{name: "message channel spelling", customer: sim.CustomerFernanda, channel: "e-mail", code: codes.InvalidArgument},
		{name: "empty channel", customer: sim.CustomerFernanda, channel: "", code: codes.InvalidArgument},
		{name: "unknown customer", customer: identity.MustNewV7(), channel: sim.ChannelChat, code: codes.NotFound},
		{name: "invalid customer id", customer: "not-a-uuid", channel: sim.ChannelChat, code: codes.InvalidArgument},
	} {
		_, err := client.UpdatePreferences(ctx, &accountv1.UpdatePreferencesRequest{CustomerId: tc.customer, Channel: tc.channel})
		if err == nil {
			t.Fatalf("%s: UpdatePreferences succeeded", tc.name)
		}
		wantCode(t, err, tc.code)
	}
	_, err = client.GetPreferences(ctx, &accountv1.GetPreferencesRequest{CustomerId: identity.MustNewV7()})
	wantCode(t, err, codes.NotFound)
	_, err = client.GetPreferences(ctx, &accountv1.GetPreferencesRequest{CustomerId: "not-a-uuid"})
	wantCode(t, err, codes.InvalidArgument)
	if stored := preferencesOf(t, client, sim.CustomerFernanda); stored != (sim.Preferences{Channel: sim.ChannelChat, Beta: true}) {
		t.Fatalf("a refusal changed the preferences: %+v", stored)
	}
	if got := events(); got != before {
		t.Fatalf("outbox rows = %d, want %d: preferences publish no event", got, before)
	}

	reseed()
	assertSeedPreferences(t, client)
}

func TestGRPCServer_Preferences(t *testing.T) {
	t.Parallel()
	memory := sim.NewMemory()
	client := startAccountServer(t, memory)
	assertPreferences(t, client, func() int { return len(memory.PendingOutbox()) }, func() {
		if err := sim.Reseed(t.Context(), memory); err != nil {
			t.Fatalf("Reseed: %v", err)
		}
	})
}

func TestGetPreferences_Refusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		store    sim.Store
		customer string
		want     error
	}{
		{name: "no store", customer: sim.CustomerFernanda},
		{name: "invalid id", store: sim.NewMemory(), customer: "x", want: identity.ErrInvalidID},
		{name: "unknown customer", store: sim.NewMemory(), customer: identity.MustNewV7(), want: sim.ErrUnknownCustomer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := sim.GetPreferences(t.Context(), tt.store, tt.customer)
			if err == nil || (tt.want != nil && !errors.Is(err, tt.want)) {
				t.Fatalf("GetPreferences error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestUpdatePreferences_Refusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		noStore  bool
		customer string
		prefs    sim.Preferences
		want     error
	}{
		{name: "no store", noStore: true, customer: sim.CustomerFernanda, prefs: sim.DefaultPreferences()},
		{name: "invalid id", customer: "x", prefs: sim.DefaultPreferences(), want: identity.ErrInvalidID},
		{name: "bad channel", customer: sim.CustomerFernanda, prefs: sim.Preferences{Channel: "sms", Beta: true}, want: sim.ErrChannel},
		{name: "unknown customer", customer: identity.MustNewV7(), prefs: sim.Preferences{Channel: sim.ChannelEmail}, want: sim.ErrUnknownCustomer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			memory := sim.NewMemory()
			var store sim.Store = memory
			if tt.noStore {
				store = nil
			}
			_, err := sim.UpdatePreferences(t.Context(), store, tt.customer, tt.prefs)
			if err == nil || (tt.want != nil && !errors.Is(err, tt.want)) {
				t.Fatalf("UpdatePreferences error = %v, want %v", err, tt.want)
			}
			if got, err := sim.GetPreferences(t.Context(), memory, sim.CustomerFernanda); err != nil || got != sim.DefaultPreferences() {
				t.Fatalf("preferences after refusal = %+v, %v", got, err)
			}
		})
	}
}

func TestUpdatePreferences_CanceledContext(t *testing.T) {
	t.Parallel()
	client := startAccountServer(t, sim.NewMemory())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := sim.UpdatePreferences(ctx, sim.NewMemory(), sim.CustomerFernanda, sim.DefaultPreferences()); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdatePreferences error = %v, want context.Canceled", err)
	}
	_, err := client.UpdatePreferences(ctx, &accountv1.UpdatePreferencesRequest{CustomerId: sim.CustomerFernanda, Channel: sim.ChannelChat})
	wantCode(t, err, codes.Canceled)
}

// TestMemory_PreferencesRollback proves a failed transaction leaves the
// previous preferences, and that PutPreferences refuses an unknown customer.
func TestMemory_PreferencesRollback(t *testing.T) {
	t.Parallel()
	memory := sim.NewMemory()
	errBoom := errors.New("boom")
	err := memory.WithTx(t.Context(), func(tx sim.Tx) error {
		if err := tx.PutPreferences(t.Context(), sim.CustomerThiago, sim.Preferences{Channel: sim.ChannelEmail, Beta: true}); err != nil {
			return err
		}
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("WithTx error = %v", err)
	}
	if got, err := sim.GetPreferences(t.Context(), memory, sim.CustomerThiago); err != nil || got != sim.DefaultPreferences() {
		t.Fatalf("preferences after rollback = %+v, %v", got, err)
	}
	err = memory.WithTx(t.Context(), func(tx sim.Tx) error {
		return tx.PutPreferences(t.Context(), identity.MustNewV7(), sim.DefaultPreferences())
	})
	if !errors.Is(err, sim.ErrUnknownCustomer) {
		t.Fatalf("PutPreferences(unknown) error = %v", err)
	}
}

// zeroPreferencesSeed is the demo seed with every account's Preferences left
// at the zero value.
func zeroPreferencesSeed() sim.Seed {
	seed := sim.DemoSeed()
	for i := range seed.Accounts {
		seed.Accounts[i].Preferences = sim.Preferences{}
	}
	return seed
}

// TestMemory_ResetZeroPreferences resets from a seed that leaves Preferences
// at the zero value: every account stores DefaultPreferences, never an empty
// channel.
func TestMemory_ResetZeroPreferences(t *testing.T) {
	t.Parallel()
	memory := sim.NewMemory()
	if _, err := sim.UpdatePreferences(t.Context(), memory, sim.CustomerFernanda, sim.Preferences{Channel: sim.ChannelEmail, Beta: true}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if err := memory.WithTx(t.Context(), func(tx sim.Tx) error {
		return tx.ResetPOV(t.Context(), zeroPreferencesSeed())
	}); err != nil {
		t.Fatalf("ResetPOV: %v", err)
	}
	for _, id := range povCustomers {
		if got, err := sim.GetPreferences(t.Context(), memory, id); err != nil || got != sim.DefaultPreferences() {
			t.Errorf("GetPreferences(%s) = %+v, %v; want the default", id, got, err)
		}
	}
}

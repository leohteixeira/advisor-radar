package sim

import (
	"context"
	"errors"
	"fmt"

	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// Contact channels a POV client may prefer. They are distinct from the
// message command channels ("chat" and "e-mail").
const (
	ChannelChat  = "chat"
	ChannelEmail = "email"
)

// ErrChannel is an UpdatePreferences channel other than ChannelChat or
// ChannelEmail.
var ErrChannel = errors.New("sim: channel must be chat or email")

// Preferences are a POV client's contact channel and beta program flag. They
// are account-sim state only: changing them writes no outbox row.
type Preferences struct {
	Channel string
	Beta    bool
}

// DefaultPreferences is what the seed stores for every POV client, and what a
// store answers for a known customer that has no preferences row yet.
func DefaultPreferences() Preferences {
	return Preferences{Channel: ChannelChat}
}

// seedPreferences is what ResetPOV stores for a seed account: a seed that
// leaves Preferences at its zero value gets DefaultPreferences, so a reset
// never stores an empty channel.
func seedPreferences(prefs Preferences) Preferences {
	if prefs == (Preferences{}) {
		return DefaultPreferences()
	}
	return prefs
}

func validChannel(channel string) bool {
	return channel == ChannelChat || channel == ChannelEmail
}

// GetPreferences reads a customer's preferences. An unknown customer is
// ErrUnknownCustomer.
func GetPreferences(ctx context.Context, store Store, customerID string) (Preferences, error) {
	if store == nil {
		return Preferences{}, errors.New("sim: store is required")
	}
	if _, err := identity.ParseV7(customerID); err != nil {
		return Preferences{}, fmt.Errorf("sim: customer: %w", err)
	}
	var prefs Preferences
	err := store.WithTx(ctx, func(tx Tx) error {
		found, ok, err := tx.GetPreferences(ctx, customerID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrUnknownCustomer
		}
		prefs = found
		return nil
	})
	if err != nil {
		return Preferences{}, err
	}
	return prefs, nil
}

// UpdatePreferences stores a customer's preferences and returns them. A bad
// channel is ErrChannel and an unknown customer ErrUnknownCustomer; neither
// writes anything. The write is serialized with the customer's commands and
// appends no outbox row, so it publishes no event.
func UpdatePreferences(ctx context.Context, store Store, customerID string, prefs Preferences) (Preferences, error) {
	if store == nil {
		return Preferences{}, errors.New("sim: store is required")
	}
	if _, err := identity.ParseV7(customerID); err != nil {
		return Preferences{}, fmt.Errorf("sim: customer: %w", err)
	}
	if !validChannel(prefs.Channel) {
		return Preferences{}, ErrChannel
	}
	err := store.WithTx(ctx, func(tx Tx) error {
		// GetPreferences takes the customer lock and proves the customer exists.
		if _, ok, err := tx.GetPreferences(ctx, customerID); err != nil {
			return err
		} else if !ok {
			return ErrUnknownCustomer
		}
		return tx.PutPreferences(ctx, customerID, prefs)
	})
	if err != nil {
		return Preferences{}, err
	}
	return prefs, nil
}

package cases

import "github.com/leohteixeira/advisor-radar/internal/event"

// DelayArgs are the broker arguments for one SLA delay message.
type DelayArgs struct {
	TTLMs                int
	DeadLetterRoutingKey string
}

// SLADelayArgs builds TTL (milliseconds) and the dead-letter routing key
// for an SLA of the given total minutes.
func SLADelayArgs(minutes int) DelayArgs {
	return DelayArgs{
		TTLMs:                minutes * 60 * 1000,
		DeadLetterRoutingKey: event.NameCaseSLABreached,
	}
}

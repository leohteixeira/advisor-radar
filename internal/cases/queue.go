package cases

import (
	"strconv"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// LegacyDelayQueue is the single SLA delay queue of the first version, which
// set the TTL per message. It stays declared so the messages already in it
// keep dead-lettering; nothing is published to it anymore (ADR 0012).
const LegacyDelayQueue = "cases.sla.delay"

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

// DelayQueue is the queue the SLA delays of one TTL are published to. RabbitMQ
// expires only the message at the head of a queue, so one queue per TTL keeps
// every queue in expiry order: a short delay never waits behind a long one.
func DelayQueue(ttlMs int) string {
	return LegacyDelayQueue + "." + strconv.Itoa(ttlMs)
}

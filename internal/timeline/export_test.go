package timeline

import "time"

// ReplayRows exposes replayRows to the external test package.
var ReplayRows = replayRows

// NewElasticStoreWithTimeouts builds a store with explicit request timeouts.
func NewElasticStoreWithTimeouts(baseURL string, document, clearing time.Duration) *ElasticStore {
	return newElasticStore(baseURL, document, clearing)
}

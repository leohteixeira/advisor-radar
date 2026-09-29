package screen

import "slices"

// inBeta reports whether the customer's stored beta flag is on. A failed
// preferences read counts as off, so that client gets the default revision.
func inBeta(snap Snapshot) bool {
	return snap.Preferences.OK() && snap.Preferences.Value.Beta
}

// servedPlan picks the revision of a screen for one Snapshot: beta, when the
// screen has a beta revision (hasBeta) and the customer is in the beta
// program, and def otherwise. The web client never chooses the revision.
func servedPlan(def, beta plan, hasBeta bool, snap Snapshot) plan {
	if hasBeta && inBeta(snap) {
		return beta
	}
	return def
}

// withBeta aligns a screen's default and beta plans so the revision can be
// chosen after the one Snapshot: both fetch the sources of either revision
// plus the preferences that decide between them. The preferences read is
// optional, never required: when it fails the client silently gets the
// default revision, and only the source span records the error.
func withBeta(def, beta plan) (plan, plan) {
	fetched := sourceUnion(def.sources, beta.sources, []Source{SourcePreferences})
	def.sources, beta.sources = fetched, fetched
	return def, beta
}

// sourceUnion returns every source in any of sets, once, in allSources order.
func sourceUnion(sets ...[]Source) []Source {
	var out []Source
	for _, src := range allSources {
		for _, set := range sets {
			if slices.Contains(set, src) {
				out = append(out, src)
				break
			}
		}
	}
	return out
}

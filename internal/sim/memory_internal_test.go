package sim

import "testing"

// TestMemoryTx_GetPreferencesDefaultWithoutEntry reads a known customer whose
// preferences entry is missing: it answers DefaultPreferences, as the pgx
// store does without a row, never a zero-value channel.
func TestMemoryTx_GetPreferencesDefaultWithoutEntry(t *testing.T) {
	t.Parallel()
	memory := NewMemory()
	delete(memory.preferences, CustomerMariana)
	got, err := GetPreferences(t.Context(), memory, CustomerMariana)
	if err != nil || got != DefaultPreferences() {
		t.Fatalf("GetPreferences = %+v, %v; want the default", got, err)
	}
}

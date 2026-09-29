package advisory

import "testing"

func TestMaxRisk(t *testing.T) {
	t.Parallel()
	tests := []struct {
		profile  string
		expected int
		wantErr  bool
	}{
		{profile: ProfileConservador, expected: 2},
		{profile: ProfileModerado, expected: 3},
		{profile: ProfileArrojado, expected: 5},
		{profile: "agressivo", wantErr: true},
		{profile: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run("profile "+tt.profile, func(t *testing.T) {
			t.Parallel()
			got, err := MaxRisk(tt.profile)
			if (err != nil) != tt.wantErr || got != tt.expected {
				t.Errorf("MaxRisk(%q) = %d, %v; want %d, error %t", tt.profile, got, err, tt.expected, tt.wantErr)
			}
		})
	}
}

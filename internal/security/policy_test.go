package security

import "testing"

func TestPolicy_Evaluate_ModerateAdvisory(t *testing.T) {
	// MODERATE was previously parsed as SeverityUnknown, causing Evaluate to
	// return Allow regardless of policy. Verify that a MODERATE advisory now
	// resolves to SeverityMedium and triggers the correct outcome.
	tests := []struct {
		name          string
		blockSeverity string
		warnSeverity  string
		want          Outcome
	}{
		{
			name:          "warn policy at MEDIUM triggers Warn",
			blockSeverity: "CRITICAL",
			warnSeverity:  "MEDIUM",
			want:          Warn,
		},
		{
			name:          "block policy at MEDIUM triggers Block",
			blockSeverity: "MEDIUM",
			warnSeverity:  "LOW",
			want:          Block,
		},
		{
			name:          "policy threshold above MEDIUM allows through",
			blockSeverity: "CRITICAL",
			warnSeverity:  "HIGH",
			want:          Allow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy := NewPolicy(tc.blockSeverity, tc.warnSeverity)
			records := []CVERecord{
				{ID: "GHSA-moderate", Severity: ParseSeverity("MODERATE")},
			}
			if got := policy.Evaluate(records); got != tc.want {
				t.Errorf("Evaluate() = %v, want %v", got, tc.want)
			}
		})
	}
}

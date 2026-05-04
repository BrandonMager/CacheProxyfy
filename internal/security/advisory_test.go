package security

import "testing"

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  Severity
	}{
		{"CRITICAL", SeverityCritical},
		{"critical", SeverityCritical},
		{"HIGH", SeverityHigh},
		{"high", SeverityHigh},
		{"MEDIUM", SeverityMedium},
		{"medium", SeverityMedium},
		{"MODERATE", SeverityMedium},
		{"moderate", SeverityMedium},
		{"LOW", SeverityLow},
		{"low", SeverityLow},
		{"NONE", SeverityUnknown},
		{"", SeverityUnknown},
		{"UNKNOWN", SeverityUnknown},
	}

	for _, tc := range tests {
		got := ParseSeverity(tc.input)
		if got != tc.want {
			t.Errorf("ParseSeverity(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

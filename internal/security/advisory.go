package security

import "strings"

// Severity represents the severity level of a CVE.
type Severity int

const (
	SeverityUnknown  Severity = iota
	SeverityLow
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityLow:
		return "LOW"
	case SeverityMedium:
		return "MEDIUM"
	case SeverityHigh:
		return "HIGH"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

func ParseSeverity(s string) Severity {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return SeverityCritical
	case "HIGH":
		return SeverityHigh
	case "MEDIUM":
		return SeverityMedium
	case "LOW":
		return SeverityLow
	default:
		return SeverityUnknown
	}
}

// CVERecord holds information about a single vulnerability returned by the OSV API.
type CVERecord struct {
	ID       string
	Summary  string
	Severity Severity
}

// Outcome is the policy decision for a set of CVE records.
type Outcome int

const (
	Allow Outcome = iota
	Warn
	Block
)

func (o Outcome) String() string {
	switch o {
	case Warn:
		return "warn"
	case Block:
		return "block"
	default:
		return "allow"
	}
}

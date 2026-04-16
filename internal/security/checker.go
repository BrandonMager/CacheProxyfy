package security

import "context"

// Checker combines Scanner and Policy into a single Check call.
// It implements the proxy.SecurityChecker interface.
type Checker struct {
	scanner *Scanner
	policy  *Policy
	enabled bool
}

func NewChecker(enabled bool, blockSeverity, warnSeverity string) *Checker {
	return &Checker{
		scanner: NewScanner(),
		policy:  NewPolicy(blockSeverity, warnSeverity),
		enabled: enabled,
	}
}

// Check scans the package for CVEs and returns the policy outcome.
// When scanning is disabled or returns an error, it fails open (Allow).
func (c *Checker) Check(ctx context.Context, ecosystem, name, version string) (Outcome, []CVERecord, error) {
	if !c.enabled {
		return Allow, nil, nil
	}
	records, err := c.scanner.Scan(ctx, ecosystem, name, version)
	if err != nil {
		return Allow, nil, err
	}
	return c.policy.Evaluate(records), records, nil
}

package security

// Policy evaluates a set of CVE records against configured severity thresholds
// and returns an Outcome (Allow, Warn, or Block).
type Policy struct {
	blockAt Severity
	warnAt  Severity
}

func NewPolicy(blockSeverity, warnSeverity string) *Policy {
	return &Policy{
		blockAt: ParseSeverity(blockSeverity),
		warnAt:  ParseSeverity(warnSeverity),
	}
}

// Evaluate returns the strictest outcome triggered by the given records.
func (p *Policy) Evaluate(records []CVERecord) Outcome {
	if len(records) == 0 {
		return Allow
	}

	var max Severity
	for _, r := range records {
		if r.Severity > max {
			max = r.Severity
		}
	}

	if p.blockAt > SeverityUnknown && max >= p.blockAt {
		return Block
	}
	if p.warnAt > SeverityUnknown && max >= p.warnAt {
		return Warn
	}
	return Allow
}

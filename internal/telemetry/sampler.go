package telemetry

import "strings"

type TraceSampler string

const (
	TraceSamplerAlwaysOn                TraceSampler = "always_on"
	TraceSamplerAlwaysOff               TraceSampler = "always_off"
	TraceSamplerTraceIDRatio            TraceSampler = "traceidratio"
	TraceSamplerParentBasedAlwaysOn     TraceSampler = "parentbased_always_on"
	TraceSamplerParentBasedAlwaysOff    TraceSampler = "parentbased_always_off"
	TraceSamplerParentBasedTraceIDRatio TraceSampler = "parentbased_traceidratio"

	DefaultTraceSampler = TraceSamplerParentBasedAlwaysOn
)

func ParseTraceSampler(value string) (TraceSampler, bool) {
	parsed := TraceSampler(strings.ToLower(strings.TrimSpace(value)))
	if !parsed.IsValid() {
		return "", false
	}

	return parsed, true
}

func (s TraceSampler) IsValid() bool {
	switch s {
	case TraceSamplerAlwaysOn,
		TraceSamplerAlwaysOff,
		TraceSamplerTraceIDRatio,
		TraceSamplerParentBasedAlwaysOn,
		TraceSamplerParentBasedAlwaysOff,
		TraceSamplerParentBasedTraceIDRatio:
		return true
	default:
		return false
	}
}

func (s TraceSampler) UsesArgument() bool {
	switch s {
	case TraceSamplerTraceIDRatio, TraceSamplerParentBasedTraceIDRatio:
		return true
	default:
		return false
	}
}

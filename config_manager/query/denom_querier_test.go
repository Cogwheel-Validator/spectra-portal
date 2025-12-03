package query

import (
	"testing"
)

func TestComputeDenomHash(t *testing.T) {
	tests := []struct {
		name     string
		trace    string
		expected string // First few chars to verify format
	}{
		{
			name:     "simple trace",
			trace:    "transfer/channel-0/uatom",
			expected: "ibc/", // Should start with ibc/
		},
		{
			name:     "multi-hop trace",
			trace:    "transfer/channel-2/transfer/channel-75/uatone",
			expected: "ibc/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeDenomHash(tt.trace)
			if result[:4] != tt.expected {
				t.Errorf("ComputeDenomHash(%q) = %q, want prefix %q", tt.trace, result, tt.expected)
			}
			// IBC denom should be ibc/ + 64 hex chars
			if len(result) != 68 { // "ibc/" (4) + 64 hex chars
				t.Errorf("ComputeDenomHash(%q) has length %d, want 68", tt.trace, len(result))
			}
		})
	}
}

func TestParseDenomTrace(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		baseDenom string
		wantHops  int
		wantFirst string
	}{
		{
			name:      "single hop",
			path:      "transfer/channel-2",
			baseDenom: "uatone",
			wantHops:  1,
			wantFirst: "channel-2",
		},
		{
			name:      "two hops",
			path:      "transfer/channel-2/transfer/channel-75",
			baseDenom: "uatone",
			wantHops:  2,
			wantFirst: "channel-2",
		},
		{
			name:      "three hops",
			path:      "transfer/channel-2/transfer/channel-75/transfer/channel-448",
			baseDenom: "uatone",
			wantHops:  3,
			wantFirst: "channel-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseDenomTrace(tt.path, tt.baseDenom)

			if result.HopCount != tt.wantHops {
				t.Errorf("ParseDenomTrace() HopCount = %d, want %d", result.HopCount, tt.wantHops)
			}
			if result.FirstHop != tt.wantFirst {
				t.Errorf("ParseDenomTrace() FirstHop = %q, want %q", result.FirstHop, tt.wantFirst)
			}
			if result.BaseDenom != tt.baseDenom {
				t.Errorf("ParseDenomTrace() BaseDenom = %q, want %q", result.BaseDenom, tt.baseDenom)
			}
			if result.Path != tt.path {
				t.Errorf("ParseDenomTrace() Path = %q, want %q", result.Path, tt.path)
			}
			// Check IBC denom is computed
			if result.IBCDenom[:4] != "ibc/" {
				t.Errorf("ParseDenomTrace() IBCDenom should start with ibc/, got %q", result.IBCDenom)
			}
		})
	}
}

func TestFilterTracesByChannel(t *testing.T) {
	traces := []DenomTraceInfo{
		{Path: "transfer/channel-2", BaseDenom: "uosmo"},
		{Path: "transfer/channel-2/transfer/channel-75", BaseDenom: "uatone"},
		{Path: "transfer/channel-4/transfer/channel-94814", BaseDenom: "uatone"},
		{Path: "transfer/channel-4", BaseDenom: "uosmo"},
		{Path: "transfer/channel-10", BaseDenom: "uaxl"},
	}

	// Filter for channel-2
	result := FilterTracesByChannel(traces, "channel-2")
	if len(result) != 2 {
		t.Errorf("FilterTracesByChannel() returned %d traces, want 2", len(result))
	}

	// Filter for channel-4
	result = FilterTracesByChannel(traces, "channel-4")
	if len(result) != 2 {
		t.Errorf("FilterTracesByChannel() returned %d traces, want 2", len(result))
	}

	// Filter for channel-10
	result = FilterTracesByChannel(traces, "channel-10")
	if len(result) != 1 {
		t.Errorf("FilterTracesByChannel() returned %d traces, want 1", len(result))
	}
}

func TestFilterDirectTraces(t *testing.T) {
	traces := []DenomTraceInfo{
		{Path: "transfer/channel-2", BaseDenom: "uosmo"},                         // Direct
		{Path: "transfer/channel-2/transfer/channel-75", BaseDenom: "uatone"},    // Multi-hop
		{Path: "transfer/channel-4", BaseDenom: "uosmo"},                         // Direct
		{Path: "transfer/channel-4/transfer/channel-94814", BaseDenom: "uatone"}, // Multi-hop
	}

	// Filter direct traces for channel-2
	result := FilterDirectTraces(traces, "channel-2")
	if len(result) != 1 {
		t.Errorf("FilterDirectTraces() returned %d traces, want 1", len(result))
	}
	if result[0].BaseDenom != "uosmo" {
		t.Errorf("FilterDirectTraces() returned wrong denom %q, want uosmo", result[0].BaseDenom)
	}

	// Filter direct traces for channel-4
	result = FilterDirectTraces(traces, "channel-4")
	if len(result) != 1 {
		t.Errorf("FilterDirectTraces() returned %d traces, want 1", len(result))
	}
}

// TestDenomHashConsistency verifies that the same trace always produces the same hash
func TestDenomHashConsistency(t *testing.T) {
	trace := "transfer/channel-94814/uatone"

	hash1 := ComputeDenomHash(trace)
	hash2 := ComputeDenomHash(trace)

	if hash1 != hash2 {
		t.Errorf("ComputeDenomHash is not consistent: %q != %q", hash1, hash2)
	}
}

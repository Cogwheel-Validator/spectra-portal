package validator

import (
	"testing"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/query"
)

func makeBlockData(ts time.Time, chainID string, hashes map[string]string, proposer string) *query.BlockData {
	bd := &query.BlockData{}
	bd.Block.Header.Time = ts
	bd.Block.Header.ChainID = chainID
	bd.Block.Header.LastCommitHash = hashes["lastCommitHash"]
	bd.Block.Header.DataHash = hashes["dataHash"]
	bd.Block.Header.ValidatorsHash = hashes["validatorsHash"]
	bd.Block.Header.NextValidatorsHash = hashes["nextValidatorsHash"]
	bd.Block.Header.ConsensusHash = hashes["consensusHash"]
	bd.Block.Header.AppHash = hashes["appHash"]
	bd.Block.Header.LastResultsHash = hashes["lastResultsHash"]
	bd.Block.Header.EvidenceHash = hashes["evidenceHash"]
	bd.Block.Header.ProposerAddress = proposer
	return bd
}

func baseHashes() map[string]string {
	return map[string]string{
		"lastCommitHash":     "lch",
		"dataHash":           "dh",
		"validatorsHash":     "vh",
		"nextValidatorsHash": "nvh",
		"consensusHash":      "ch",
		"appHash":            "ah",
		"lastResultsHash":    "lrh",
		"evidenceHash":       "eh",
	}
}

func makeRestValidity(url string, pts int, valid bool, blocks map[int]*query.BlockData) RestApiValidity {
	return RestApiValidity{
		Endpoint:  input.APIEndpoint{URL: url},
		points:    pts,
		valid:     valid,
		blockData: blocks,
	}
}

func TestValidateBlockData_PenalizesMissingBlockData(t *testing.T) {
	ts := time.Now()
	h := 100

	a := makeRestValidity("https://a", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "chain-A", baseHashes(), "prop"),
	})
	b := makeRestValidity("https://b", 100, true, map[int]*query.BlockData{
		h: nil,
	})

	m := map[string]RestApiValidity{"a": a, "b": b}

	validateBlockData(&m)

	if m["a"].GetPoints() != 100 {
		t.Fatalf("endpoint a points = %d, want 100", m["a"].GetPoints())
	}
	if got := m["b"].GetPoints(); got != 90 {
		t.Fatalf("endpoint b points = %d, want 90 (missing block penalty)", got)
	}
	if !m["b"].IsValid() {
		t.Fatalf("endpoint b should remain valid with 90 points")
	}
}

func TestValidateBlockData_PenalizesMismatchedHeaders(t *testing.T) {
	ts := time.Now()
	h := 101
	hashes := baseHashes()

	// Endpoint A has the consensus values
	a := makeRestValidity("https://a", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "chain-A", hashes, "prop"),
	})
	// Endpoint B mismatches chainId and appHash
	hashesB := baseHashes()
	hashesB["appHash"] = "ah-mismatch"
	b := makeRestValidity("https://b", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "chain-B", hashesB, "prop"),
	})

	m := map[string]RestApiValidity{"a": a, "b": b}
	validateBlockData(&m)

	if m["a"].GetPoints() != 100 {
		t.Fatalf("endpoint a points = %d, want 100", m["a"].GetPoints())
	}
	if got := m["b"].GetPoints(); got != 80 {
		t.Fatalf("endpoint b points = %d, want 80 (two mismatches)", got)
	}
}

func TestValidateBlockData_MarksInvalidBelowThreshold(t *testing.T) {
	ts := time.Now()
	h := 102

	// Build three endpoints to set consensus opposite to c
	hashes := baseHashes()
	a := makeRestValidity("https://a", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "C", hashes, "P"),
	})
	b := makeRestValidity("https://b", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "C", hashes, "P"),
	})

	// Endpoint C mismatches 5 fields so it drops to 50 and becomes invalid
	hashesC := baseHashes()
	hashesC["dataHash"] = "x1"
	hashesC["validatorsHash"] = "x2"
	hashesC["nextValidatorsHash"] = "x3"
	hashesC["consensusHash"] = "x4"
	hashesC["appHash"] = "x5"

	c := makeRestValidity("https://c", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "D", hashesC, "Q"), // also mismatches chainID and proposer -> 7 mismatches
	})

	m := map[string]RestApiValidity{"a": a, "b": b, "c": c}
	validateBlockData(&m)

	// Expect endpoint c to be marked invalid: 7 mismatches => 30 points, definitely below 60
	if m["c"].GetPoints() >= 60 || m["c"].IsValid() {
		t.Fatalf("endpoint c should be invalid with points < 60, got points=%d valid=%v", m["c"].GetPoints(), m["c"].IsValid())
	}
}

func TestValidateBlockData_SkipsAlreadyInvalidEndpoints(t *testing.T) {
	ts := time.Now()
	h := 103

	// Valid endpoint to form consensus
	a := makeRestValidity("https://a", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "X", baseHashes(), "P"),
	})

	// Invalid endpoint should be skipped entirely and remain unchanged
	bad := makeRestValidity("https://bad", 55, false, map[int]*query.BlockData{
		h: makeBlockData(ts, "Y", baseHashes(), "Q"),
	})

	m := map[string]RestApiValidity{"a": a, "bad": bad}
	validateBlockData(&m)

	if m["bad"].GetPoints() != 55 {
		t.Fatalf("invalid endpoint should be skipped; points changed to %d", m["bad"].GetPoints())
	}
	if m["bad"].IsValid() {
		t.Fatalf("invalid endpoint should remain invalid")
	}
}

func TestValidateBlockData_ConsensusUsesMostCommon(t *testing.T) {
	ts := time.Now()
	h := 104
	hashes := baseHashes()

	// Two endpoints agree on chainId "A", one disagrees with "B"
	e1 := makeRestValidity("https://e1", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "A", hashes, "P"),
	})
	e2 := makeRestValidity("https://e2", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "A", hashes, "P"),
	})
	e3 := makeRestValidity("https://e3", 100, true, map[int]*query.BlockData{
		h: makeBlockData(ts, "B", hashes, "P"),
	})

	m := map[string]RestApiValidity{"e1": e1, "e2": e2, "e3": e3}
	validateBlockData(&m)

	if m["e1"].GetPoints() != 100 || m["e2"].GetPoints() != 100 {
		t.Fatalf("endpoints e1/e2 should match consensus and keep 100 points, got %d/%d", m["e1"].GetPoints(), m["e2"].GetPoints())
	}
	if got := m["e3"].GetPoints(); got != 90 {
		t.Fatalf("endpoint e3 should be penalized 10 for chainId mismatch, got %d", got)
	}
}

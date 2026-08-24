//go:build e2e

// Package e2e exercises a running pathfinder RPC server over the network.
// It is excluded from the normal build and test run by the "e2e" build tag
// and requires E2E_PATHFINDER_URL to point at a real server; run it with:
//
//	go test -tags=e2e ./pathfinder/e2e/... -v
package e2e

import (
	"net/http"
	"os"
	"testing"
	"time"

	v2betaconnect "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta/v2betaconnect"
)

// BaseURL returns the pathfinder RPC base URL under test, e.g.
// https://pathfinder.example.com, from E2E_PATHFINDER_URL.
func BaseURL() string {
	return os.Getenv("E2E_PATHFINDER_URL")
}

// RequireBaseURL skips the current test when E2E_PATHFINDER_URL isn't set,
// so these tests never run as a side effect of the normal unit-test suite.
func RequireBaseURL(t *testing.T) string {
	t.Helper()
	url := BaseURL()
	if url == "" {
		t.Skip("E2E_PATHFINDER_URL not set; skipping e2e test")
	}
	return url
}

// NewFindPathClient builds a v2beta FindPathService client against the
// configured base URL, skipping the test if none is configured.
func NewFindPathClient(t *testing.T) v2betaconnect.FindPathServiceClient {
	t.Helper()
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return v2betaconnect.NewFindPathServiceClient(httpClient, RequireBaseURL(t))
}

// NewQueryClient builds a v2beta PathfinderQueryService client against the
// configured base URL, skipping the test if none is configured.
func NewQueryClient(t *testing.T) v2betaconnect.PathfinderQueryServiceClient {
	t.Helper()
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return v2betaconnect.NewPathfinderQueryServiceClient(httpClient, RequireBaseURL(t))
}

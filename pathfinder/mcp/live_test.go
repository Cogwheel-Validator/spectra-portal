//go:build live

package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/config"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp/cosmostx"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	"github.com/zeebo/assert"
)

// Run with: LIVE_CHAIN=cosmoshub-4 LIVE_TX=<hash> go test -tags live -run TestLiveTrack ./pathfinder/mcp/
// Uses the generated pathfinder config; endpoints come from LIVE_REST_<CHAIN_ID> env vars
// (e.g. LIVE_REST_cosmoshub-4=https://...), because generated configs may predate the endpoint fields.
func TestLiveTrack(t *testing.T) {
	chainID, tx := os.Getenv("LIVE_CHAIN"), os.Getenv("LIVE_TX")
	if chainID == "" || tx == "" {
		t.Skip("set LIVE_CHAIN and LIVE_TX")
	}
	chains, err := config.NewChainConfigLoader().LoadFromFile("../../generated_configs/pathfinder_config.toml")
	assert.NoError(t, err)
	for i := range chains {
		if u := os.Getenv("LIVE_REST_" + chains[i].Id); u != "" {
			chains[i].Rests = []string{u}
		}
	}
	idx := routeindex.NewRouteIndex()
	assert.NoError(t, idx.BuildIndex(chains))
	tl := newTools(common.Deps{Pathfinder: router.NewPathfinder(chains, idx, nil)}, Options{})
	res, err := tl.trackerFn.track(context.Background(), chainID, tx, true)
	assert.NoError(t, err)
	b, _ := json.MarshalIndent(res, "", " ")
	t.Log(string(b))
}

// Run with: LIVE_SIGNER=<cosmos addr> LIVE_PUBKEY=<base64> LIVE_REST_cosmoshub-4=<url> go test -tags live -run TestLiveSignDoc ./pathfinder/mcp/
// Read-only: simulates a transfer for the given account; nothing is signed or sent.
func TestLiveSignDoc(t *testing.T) {
	signer, pub := os.Getenv("LIVE_SIGNER"), os.Getenv("LIVE_PUBKEY")
	if signer == "" || pub == "" {
		t.Skip("set LIVE_SIGNER and LIVE_PUBKEY")
	}
	chains, err := config.NewChainConfigLoader().LoadFromFile("../../generated_configs/pathfinder_config.toml")
	assert.NoError(t, err)
	for i := range chains {
		if u := os.Getenv("LIVE_REST_" + chains[i].Id); u != "" {
			chains[i].Rests = []string{u}
		}
	}
	idx := routeindex.NewRouteIndex()
	assert.NoError(t, idx.BuildIndex(chains))
	tl := newTools(common.Deps{Pathfinder: router.NewPathfinder(chains, idx, nil)}, Options{})

	msg := &cosmostx.MsgTransfer{
		SourcePort: "transfer", SourceChannel: "channel-141", Token: cosmostx.Coin{Denom: "uatom", Amount: "1"},
		Sender: signer, Receiver: os.Getenv("LIVE_RECEIVER"),
		TimeoutTimestamp: uint64(tl.now().Add(10 * time.Minute).UnixNano()),
	}
	for _, mode := range []string{"direct", "amino"} {
		out, err := tl.buildSignDoc(context.Background(), signDocInput{
			ChainID: "cosmoshub-4", SignerAddress: signer, PublicKey: pub, SignMode: mode,
			Messages: []messageIn{{TypeURL: msg.TypeURL(), ValueBase64: base64.StdEncoding.EncodeToString(msg.Marshal())}},
		})
		if err != nil {
			t.Logf("%s: %v", mode, err) // simulation may legitimately fail (e.g. empty balance)
			continue
		}
		b, _ := json.MarshalIndent(out, "", " ")
		t.Logf("%s: %s", mode, b)
	}
}

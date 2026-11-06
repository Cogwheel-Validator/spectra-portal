package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp/cosmostx"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/zeebo/assert"
)

const (
	hooksContract = "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u"
	ibcAtom       = "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2"
)

func testAddr(t *testing.T, prefix string, seed byte) string {
	t.Helper()
	raw := make([]byte, 20)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	conv, err := bech32.ConvertBits(raw, 8, 5, true)
	assert.NoError(t, err)
	a, err := bech32.Encode(prefix, conv)
	assert.NoError(t, err)
	return a
}

// fakeLCD is a minimal chain REST API.
type fakeLCD struct {
	mu       sync.Mutex
	srv      *httptest.Server
	handlers map[string]func(w http.ResponseWriter, r *http.Request)
	hits     []string
}

func newFakeLCD(t *testing.T) *fakeLCD {
	t.Helper()
	f := &fakeLCD{handlers: map[string]func(http.ResponseWriter, *http.Request){}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits = append(f.hits, r.Method+" "+r.URL.Path)
		h := f.handlers[r.URL.Path]
		f.mu.Unlock()
		if h == nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
			return
		}
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLCD) json(path string, v any) {
	f.handlers[path] = func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func testTools(t *testing.T, hub, osmo *fakeLCD, opts Options) *tools {
	t.Helper()
	chains := []routeindex.PathfinderChain{
		{
			Name: "Cosmos Hub", Id: "cosmoshub-4", Bech32Prefix: "cosmos", Slip44: 118, AccountType: "cosmos",
			Rests: []string{hub.srv.URL},
			Routes: []routeindex.BasicRoute{{
				ToChain: "Osmosis", ToChainId: "osmosis-1", ChannelId: "channel-141", PortId: "transfer",
				AllowedTokens: map[string]routeindex.TokenInfo{
					"uatom": {ChainDenom: "uatom", IbcDenom: ibcAtom, BaseDenom: "uatom", OriginChain: "cosmoshub-4", Symbol: "ATOM", Decimals: 6},
				},
			}},
			NativeTokens: []routeindex.TokenInfo{{ChainDenom: "uatom", BaseDenom: "uatom", OriginChain: "cosmoshub-4", Symbol: "ATOM", Decimals: 6}},
		},
		{
			Name: "Osmosis", Id: "osmosis-1", Bech32Prefix: "osmo", Slip44: 118, AccountType: "cosmos",
			Broker: true, BrokerId: "osmosis-sqs", HasPFM: true, IBCHooksContract: hooksContract,
			Rests: []string{osmo.srv.URL},
			Routes: []routeindex.BasicRoute{{
				ToChain: "Cosmos Hub", ToChainId: "cosmoshub-4", ChannelId: "channel-0", PortId: "transfer",
				AllowedTokens: map[string]routeindex.TokenInfo{
					ibcAtom: {ChainDenom: ibcAtom, IbcDenom: "uatom", BaseDenom: "uatom", OriginChain: "cosmoshub-4", Symbol: "ATOM", Decimals: 6},
				},
			}},
			NativeTokens: []routeindex.TokenInfo{{ChainDenom: "uosmo", BaseDenom: "uosmo", OriginChain: "osmosis-1", Symbol: "OSMO", Decimals: 6}},
		},
	}
	idx := routeindex.NewRouteIndex()
	assert.NoError(t, idx.BuildIndex(chains))
	pf := router.NewPathfinder(chains, idx, nil)
	deps := common.Deps{Pathfinder: pf}
	deps.DenomResolver = newResolver(chains, idx)
	tl := newTools(deps, opts)
	tl.now = func() time.Time { return time.Unix(1_760_000_000, 0) }
	tl.trackerFn.now = tl.now
	return tl
}

func TestBuildDirectTransfer(t *testing.T) {
	tl := testTools(t, newFakeLCD(t), newFakeLCD(t), Options{})
	hub, osmo := testAddr(t, "cosmos", 1), testAddr(t, "osmo", 1)
	res, err := tl.build(context.Background(), buildInput{
		ChainFrom: "cosmoshub-4", TokenFrom: "uatom", AmountIn: "1000000", ChainTo: "osmosis-1",
		Addresses: []addressInput{{ChainID: "cosmoshub-4", Address: hub}, {ChainID: "osmosis-1", Address: osmo}},
	})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, len(res.Steps), 1)
	st := res.Steps[0]
	assert.Equal(t, st.ChainID, "cosmoshub-4")
	assert.Equal(t, st.Signer, hub)
	assert.Equal(t, len(st.Messages), 1)
	assert.NotNil(t, st.Messages[0].Amino)

	raw, err := base64.StdEncoding.DecodeString(st.Messages[0].ValueBase64)
	assert.NoError(t, err)
	m, err := cosmostx.ParseMsg(st.Messages[0].TypeURL, raw)
	assert.NoError(t, err)
	tr := m.(*cosmostx.MsgTransfer)
	assert.Equal(t, tr.Receiver, osmo)
	assert.Equal(t, tr.SourceChannel, "channel-141")
	assert.Equal(t, tr.Token.Denom, "uatom")
	assert.Equal(t, tr.Token.Amount, "1000000")
	assert.Equal(t, tr.TimeoutTimestamp, uint64(1_760_000_000+600)*1_000_000_000)
}

func TestBuildRejectsBadInput(t *testing.T) {
	tl := testTools(t, newFakeLCD(t), newFakeLCD(t), Options{})
	_, err := tl.build(context.Background(), buildInput{ChainFrom: "cosmoshub-4", TokenFrom: "uatom", AmountIn: "1", ChainTo: "osmosis-1"})
	assert.Error(t, err) // no addresses
	_, err = tl.build(context.Background(), buildInput{ChainFrom: "cosmoshub-4", TokenFrom: "uatom", AmountIn: "1", ChainTo: "osmosis-1",
		Addresses: []addressInput{{ChainID: "cosmoshub-4", Address: testAddr(t, "cosmos", 1)}, {ChainID: "osmosis-1", Address: testAddr(t, "osmo", 1)}}, Mode: "yolo"})
	assert.Error(t, err)
	_, err = tl.build(context.Background(), buildInput{ChainFrom: "cosmoshub-4", TokenFrom: "uatom", AmountIn: "1", ChainTo: "osmosis-1",
		Addresses: []addressInput{{ChainID: "cosmoshub-4", Address: testAddr(t, "cosmos", 1)}, {ChainID: "osmosis-1", Address: testAddr(t, "osmo", 1)}}, TimeoutMinutes: 999})
	assert.Error(t, err)
}

func TestApplySlippage(t *testing.T) {
	got, err := applySlippage("1000000", 100)
	assert.NoError(t, err)
	assert.Equal(t, got, "990000")
	got, err = applySlippage("99999999999999999999999", 50) // beyond int64
	assert.NoError(t, err)
	assert.Equal(t, got, "99499999999999999999999")
	_, err = applySlippage("abc", 1)
	assert.Error(t, err)
	_, err = applySlippage("1", 10001)
	assert.Error(t, err)
}

// validTestPubKey returns the compressed public key of private key 1 (the
// secp256k1 generator point), a real on-curve key.
func validTestPubKey() []byte {
	var priv secp256k1.ModNScalar
	priv.SetInt(1)
	return secp256k1.NewPrivateKey(&priv).PubKey().SerializeCompressed()
}

func TestBuildSignDocRejectsOffCurveKey(t *testing.T) {
	tl, in, _ := signDocFixture(t)
	bad := make([]byte, 33)
	bad[0] = 2
	for i := 1; i < 33; i++ {
		bad[i] = byte(i)
	}
	in.PublicKey = base64.StdEncoding.EncodeToString(bad)
	_, err := tl.buildSignDoc(context.Background(), in)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not a valid secp256k1 public key"))
}

func TestAddressFromPubKeyKnownVector(t *testing.T) {
	// Pubkey of private key 1 -> Hash160 751e76e8199196d454941c45d1b3a323f1433bd6
	// (the well-known Bitcoin value), bech32-encoded with the cosmos prefix.
	addr, err := addressFromPubKey("cosmos", validTestPubKey())
	assert.NoError(t, err)
	_, data, err := bech32.Decode(addr)
	assert.NoError(t, err)
	raw, err := bech32.ConvertBits(data, 5, 8, false)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%x", raw), "751e76e8199196d454941c45d1b3a323f1433bd6")
}

func signDocFixture(t *testing.T) (*tools, signDocInput, *fakeLCD) {
	t.Helper()
	hubLCD := newFakeLCD(t)
	tl := testTools(t, hubLCD, newFakeLCD(t), Options{})
	pub := validTestPubKey()
	signer, err := addressFromPubKey("cosmos", pub)
	assert.NoError(t, err)
	msg := &cosmostx.MsgTransfer{
		SourcePort: "transfer", SourceChannel: "channel-141", Token: cosmostx.Coin{Denom: "uatom", Amount: "5"},
		Sender: signer, Receiver: testAddr(t, "osmo", 1), TimeoutTimestamp: 42,
	}
	hubLCD.json("/cosmos/auth/v1beta1/accounts/"+signer, map[string]any{"account": map[string]any{"account_number": "77", "sequence": "9"}})
	hubLCD.json("/cosmos/base/node/v1beta1/config", map[string]any{"minimum_gas_price": "0.005uatom"})
	hubLCD.json("/cosmos/tx/v1beta1/simulate", map[string]any{"gas_info": map[string]any{"gas_used": "100000"}})
	return tl, signDocInput{
		ChainID: "cosmoshub-4", SignerAddress: signer, PublicKey: base64.StdEncoding.EncodeToString(pub),
		Messages: []messageIn{{TypeURL: msg.TypeURL(), ValueBase64: base64.StdEncoding.EncodeToString(msg.Marshal())}},
	}, hubLCD
}

func TestBuildSignDocDirectThenAssembleAndBroadcast(t *testing.T) {
	tl, in, hubLCD := signDocFixture(t)
	out, err := tl.buildSignDoc(context.Background(), in)
	assert.NoError(t, err)
	assert.Equal(t, out.AccountNumber, "77")
	assert.Equal(t, out.Sequence, "9")
	assert.Equal(t, out.GasLimit, uint64(140000)) // 100000 * 1.4
	assert.Equal(t, out.Fee.Amount[0].Denom, "uatom")
	assert.Equal(t, out.Fee.Amount[0].Amount, "700") // ceil(140000 * 0.005)
	assert.NotEqual(t, out.SignDocBase64, "")
	assert.Equal(t, out.AminoSignDocCanonical, "")

	// Assemble with a dummy signature and decode it back.
	body, _ := base64.StdEncoding.DecodeString(out.BodyBytesBase64)
	auth, _ := base64.StdEncoding.DecodeString(out.AuthInfoBytesBase64)
	tx := cosmostx.BuildTxRaw(body, auth, make([]byte, 63), []byte{1})
	_ = tx
	sig := make([]byte, 64)
	sig[0] = 1
	raw := cosmostx.BuildTxRaw(body, auth, sig)
	dec, err := cosmostx.DecodeTx(raw)
	assert.NoError(t, err)
	assert.Equal(t, dec.Fee.GasLimit, uint64(140000))
	assert.Equal(t, dec.Signers[0].Sequence, "9")
	assert.Equal(t, dec.Signers[0].SignMode, "direct")

	// broadcast is disabled by default, so the tool must not exist and the
	// method must still enforce the allowlist when enabled.
	hubLCD.handlers["/cosmos/tx/v1beta1/txs"] = func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.Method, http.MethodPost)
		var b map[string]string
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&b))
		assert.Equal(t, b["mode"], "BROADCAST_MODE_SYNC")
		_ = json.NewEncoder(w).Encode(map[string]any{"tx_response": map[string]any{"txhash": dec.TxHash, "code": 0}})
	}
	res, err := tl.broadcast(context.Background(), broadcastInput{ChainID: "cosmoshub-4", TxBytes: base64.StdEncoding.EncodeToString(raw)})
	assert.NoError(t, err)
	assert.Equal(t, res["accepted"], true)
	assert.Equal(t, res["tx_hash"], dec.TxHash)
}

func TestBuildSignDocAmino(t *testing.T) {
	tl, in, _ := signDocFixture(t)
	in.SignMode = "amino"
	out, err := tl.buildSignDoc(context.Background(), in)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(out.AminoSignDocCanonical, `"type":"cosmos-sdk/MsgTransfer"`))
	assert.True(t, strings.Contains(out.AminoSignDocCanonical, `"account_number":"77"`))
	assert.Equal(t, out.SignDocBase64, "")
}

func TestBuildSignDocSafetyChecks(t *testing.T) {
	tl, in, _ := signDocFixture(t)

	// Public key that does not match the signer address.
	bad := in
	bad.SignerAddress = testAddr(t, "cosmos", 9)
	_, err := tl.buildSignDoc(context.Background(), bad)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "public key belongs to"))

	// Message whose sender is somebody else.
	other := &cosmostx.MsgTransfer{SourcePort: "transfer", SourceChannel: "channel-1", Token: cosmostx.Coin{Denom: "uatom", Amount: "1"},
		Sender: testAddr(t, "cosmos", 5), Receiver: testAddr(t, "osmo", 1)}
	bad = in
	bad.Messages = []messageIn{{TypeURL: other.TypeURL(), ValueBase64: base64.StdEncoding.EncodeToString(other.Marshal())}}
	_, err = tl.buildSignDoc(context.Background(), bad)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "can only be signed by its senders"))

	// Unknown sign mode, bad key, no messages.
	bad = in
	bad.SignMode = "textual"
	_, err = tl.buildSignDoc(context.Background(), bad)
	assert.Error(t, err)
	bad = in
	bad.PublicKey = "AAAA"
	_, err = tl.buildSignDoc(context.Background(), bad)
	assert.Error(t, err)
	bad = in
	bad.Messages = nil
	_, err = tl.buildSignDoc(context.Background(), bad)
	assert.Error(t, err)
}

func TestBuildSignDocUnfundedAccountAndFailedSimulation(t *testing.T) {
	tl, in, hubLCD := signDocFixture(t)
	delete(hubLCD.handlers, "/cosmos/auth/v1beta1/accounts/"+in.SignerAddress)
	_, err := tl.buildSignDoc(context.Background(), in)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "does not exist"))

	hubLCD.json("/cosmos/auth/v1beta1/accounts/"+in.SignerAddress, map[string]any{"account": map[string]any{"account_number": "1", "sequence": "0"}})
	hubLCD.handlers["/cosmos/tx/v1beta1/simulate"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"insufficient funds"}`))
	}
	_, err = tl.buildSignDoc(context.Background(), in)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "insufficient funds"))
}

func TestBroadcastAllowlist(t *testing.T) {
	hub, osmo := newFakeLCD(t), newFakeLCD(t)
	tl := testTools(t, hub, osmo, Options{Broadcast: true})
	pub := validTestPubKey()
	auth, err := cosmostx.BuildAuthInfo(cosmostx.Signer{PubKey: pub, Sequence: 1}, cosmostx.Fee{GasLimit: 1})
	assert.NoError(t, err)
	mk := func(msgs []cosmostx.Msg, sig []byte) string {
		return base64.StdEncoding.EncodeToString(cosmostx.BuildTxRaw(cosmostx.BuildTxBody(msgs, "", 0), auth, sig))
	}
	sig := make([]byte, 64)
	sig[0] = 1
	transfer := &cosmostx.MsgTransfer{SourcePort: "transfer", SourceChannel: "channel-0", Token: cosmostx.Coin{Denom: "u", Amount: "1"}, Sender: "a", Receiver: "b"}

	// Unsigned.
	_, err = tl.broadcast(context.Background(), broadcastInput{ChainID: "osmosis-1", TxBytes: mk([]cosmostx.Msg{transfer}, nil)})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not fully signed"))

	// A message type outside the allowlist (bank send).
	bank := &cosmostx.RawMsg{URL: "/cosmos.bank.v1beta1.MsgSend", Value: []byte{0x0a, 0x01, 'a'}}
	_, err = tl.broadcast(context.Background(), broadcastInput{ChainID: "osmosis-1", TxBytes: mk([]cosmostx.Msg{bank}, sig)})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not allowed"))

	// Smuggling a disallowed message next to an allowed one.
	_, err = tl.broadcast(context.Background(), broadcastInput{ChainID: "osmosis-1", TxBytes: mk([]cosmostx.Msg{transfer, bank}, sig)})
	assert.Error(t, err)

	// Wasm call to an arbitrary contract.
	evil := &cosmostx.MsgExecuteContract{Sender: "a", Contract: "osmo1evil", Msg: json.RawMessage(`{}`)}
	_, err = tl.broadcast(context.Background(), broadcastInput{ChainID: "osmosis-1", TxBytes: mk([]cosmostx.Msg{evil}, sig)})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "entry-point contract"))

	// Wasm call on a chain with no configured entry point.
	_, err = tl.broadcast(context.Background(), broadcastInput{ChainID: "cosmoshub-4", TxBytes: mk([]cosmostx.Msg{&cosmostx.MsgExecuteContract{Sender: "a", Contract: hooksContract, Msg: json.RawMessage(`{}`)}}, sig)})
	assert.Error(t, err)

	// Allowed contract passes validation and reaches the node.
	osmo.handlers["/cosmos/tx/v1beta1/txs"] = func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tx_response": map[string]any{"txhash": "H", "code": 13, "raw_log": "insufficient fee"}})
	}
	good := &cosmostx.MsgExecuteContract{Sender: "a", Contract: hooksContract, Msg: json.RawMessage(`{"swap_and_action":{}}`)}
	res, err := tl.broadcast(context.Background(), broadcastInput{ChainID: "osmosis-1", TxBytes: mk([]cosmostx.Msg{good}, sig)})
	assert.NoError(t, err)
	assert.Equal(t, res["accepted"], false) // node rejection is reported, not hidden

	// Garbage and oversize input.
	_, err = tl.broadcast(context.Background(), broadcastInput{ChainID: "osmosis-1", TxBytes: "!!!"})
	assert.Error(t, err)
	_, err = tl.broadcast(context.Background(), broadcastInput{ChainID: "nope", TxBytes: mk([]cosmostx.Msg{transfer}, sig)})
	assert.Error(t, err)
	_, err = tl.broadcast(context.Background(), broadcastInput{ChainID: "osmosis-1", TxBytes: base64.StdEncoding.EncodeToString(make([]byte, maxTxBytes+1))})
	assert.Error(t, err)
}

func sendPacketEvent(seq, srcCh, dstCh string, timeoutNs int64) map[string]any {
	attr := func(k, v string) map[string]any { return map[string]any{"key": k, "value": v} }
	return map[string]any{"type": "send_packet", "attributes": []any{
		attr("packet_sequence", seq), attr("packet_src_port", "transfer"), attr("packet_src_channel", srcCh),
		attr("packet_dst_port", "transfer"), attr("packet_dst_channel", dstCh),
		attr("packet_timeout_timestamp", itoa(timeoutNs)),
	}}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestTrackTransferStates(t *testing.T) {
	hub, osmo := newFakeLCD(t), newFakeLCD(t)
	tl := testTools(t, hub, osmo, Options{})
	future := tl.now().Add(time.Hour).UnixNano()
	past := tl.now().Add(-time.Hour).UnixNano()

	hub.json("/cosmos/tx/v1beta1/txs/AAAA", map[string]any{"tx_response": map[string]any{
		"height": "10", "txhash": "AAAA", "code": 0, "events": []any{sendPacketEvent("5", "channel-141", "channel-0", future)}}})
	commit := "/ibc/core/channel/v1/channels/channel-141/ports/transfer/packet_commitments/5"
	receipt := "/ibc/core/channel/v1/channels/channel-0/ports/transfer/packet_receipts/5"

	// 1) In flight: committed, not received.
	hub.json(commit, map[string]any{"commitment": "abc"})
	osmo.json(receipt, map[string]any{"received": false})
	res, err := tl.trackerFn.track(context.Background(), "cosmoshub-4", "aaaa", true)
	assert.NoError(t, err)
	assert.Equal(t, res.Status, "in_progress")
	assert.Equal(t, res.Packets[0].Status, pktInFlight)
	assert.Equal(t, res.Packets[0].DestChain, "osmosis-1")

	// 2) Past its timeout and still unreceived.
	hub.json("/cosmos/tx/v1beta1/txs/AAAA", map[string]any{"tx_response": map[string]any{
		"height": "10", "txhash": "AAAA", "code": 0, "events": []any{sendPacketEvent("5", "channel-141", "channel-0", past)}}})
	res, _ = tl.trackerFn.track(context.Background(), "cosmoshub-4", "AAAA", true)
	assert.Equal(t, res.Packets[0].Status, pktExpired)

	// 3) Timed out and refunded: commitment gone, never received.
	delete(hub.handlers, commit)
	res, _ = tl.trackerFn.track(context.Background(), "cosmoshub-4", "AAAA", true)
	assert.Equal(t, res.Status, "timed_out")

	// 4) Received and acknowledged.
	osmo.json(receipt, map[string]any{"received": true})
	res, _ = tl.trackerFn.track(context.Background(), "cosmoshub-4", "AAAA", false)
	assert.Equal(t, res.Status, "completed")
	assert.Equal(t, res.Packets[0].Status, pktDelivered)

	// 5) Received, ack still travelling back.
	hub.json(commit, map[string]any{"commitment": "abc"})
	res, _ = tl.trackerFn.track(context.Background(), "cosmoshub-4", "AAAA", false)
	assert.Equal(t, res.Packets[0].Status, pktReceived)
	assert.Equal(t, res.Status, "completed")
}

func TestTrackTransferEdgeCases(t *testing.T) {
	hub, osmo := newFakeLCD(t), newFakeLCD(t)
	tl := testTools(t, hub, osmo, Options{})

	res, err := tl.trackerFn.track(context.Background(), "cosmoshub-4", "NOPE", true)
	assert.NoError(t, err)
	assert.Equal(t, res.Status, "not_found")

	hub.json("/cosmos/tx/v1beta1/txs/FAIL", map[string]any{"tx_response": map[string]any{"txhash": "FAIL", "code": 5, "raw_log": "insufficient funds"}})
	res, _ = tl.trackerFn.track(context.Background(), "cosmoshub-4", "FAIL", true)
	assert.Equal(t, res.Status, "tx_failed")
	assert.True(t, strings.Contains(res.Summary, "insufficient funds"))

	hub.json("/cosmos/tx/v1beta1/txs/SWAP", map[string]any{"tx_response": map[string]any{"txhash": "SWAP", "code": 0, "events": []any{}}})
	res, _ = tl.trackerFn.track(context.Background(), "cosmoshub-4", "SWAP", true)
	assert.Equal(t, res.Status, "no_ibc_packets")

	_, err = tl.trackerFn.track(context.Background(), "nope-1", "X", true)
	assert.Error(t, err)
}

func TestTrackFollowsForwardedPacketsAndAckErrors(t *testing.T) {
	hub, osmo := newFakeLCD(t), newFakeLCD(t)
	tl := testTools(t, hub, osmo, Options{})
	future := tl.now().Add(time.Hour).UnixNano()
	attr := func(k, v string) map[string]any { return map[string]any{"key": k, "value": v} }

	hub.json("/cosmos/tx/v1beta1/txs/H1", map[string]any{"tx_response": map[string]any{
		"txhash": "H1", "code": 0, "events": []any{sendPacketEvent("5", "channel-141", "channel-0", future)}}})
	hub.json("/ibc/core/channel/v1/channels/channel-141/ports/transfer/packet_commitments/5", map[string]any{"commitment": "x"})
	osmo.json("/ibc/core/channel/v1/channels/channel-0/ports/transfer/packet_receipts/5", map[string]any{"received": true})

	// recv tx on osmosis forwards a new packet back to the hub (seq 8), still unreceived.
	osmo.json("/cosmos/tx/v1beta1/txs", map[string]any{"tx_responses": []any{map[string]any{
		"txhash": "RECV1", "code": 0, "events": []any{sendPacketEvent("8", "channel-0", "channel-141", future)}}}})
	osmo.json("/ibc/core/channel/v1/channels/channel-0/ports/transfer/packet_commitments/8", map[string]any{"commitment": "y"})
	hub.json("/ibc/core/channel/v1/channels/channel-141/ports/transfer/packet_receipts/8", map[string]any{"received": false})

	res, err := tl.trackerFn.track(context.Background(), "cosmoshub-4", "H1", true)
	assert.NoError(t, err)
	assert.Equal(t, res.Status, "in_progress")
	root := res.Packets[0]
	assert.Equal(t, root.RecvTx, "RECV1")
	assert.Equal(t, len(root.Forwarded), 1)
	assert.Equal(t, root.Forwarded[0].SourceChain, "osmosis-1")
	assert.Equal(t, root.Forwarded[0].Status, pktInFlight)

	// The destination returned an error acknowledgement: funds are refunded.
	osmo.json("/cosmos/tx/v1beta1/txs", map[string]any{"tx_responses": []any{map[string]any{
		"txhash": "RECV1", "code": 0, "events": []any{map[string]any{"type": "write_acknowledgement", "attributes": []any{
			attr("packet_sequence", "5"), attr("packet_src_channel", "channel-141"), attr("packet_ack", `{"error":"ABCI code: 4: invalid"}`)}}}}}})
	res, _ = tl.trackerFn.track(context.Background(), "cosmoshub-4", "H1", true)
	assert.Equal(t, res.Status, "failed")
	assert.Equal(t, res.Packets[0].Status, pktFailed)
}

func TestSDKVersionQueryStyle(t *testing.T) {
	assert.True(t, sdkAtLeast050("v0.50.9"))
	assert.True(t, sdkAtLeast050("0.53.0"))
	assert.True(t, sdkAtLeast050(""))
	assert.True(t, sdkAtLeast050("v1.0.0"))
	assert.False(t, sdkAtLeast050("v0.47.13"))
	assert.False(t, sdkAtLeast050("v0.45.1"))
}

func TestNoEndpointsIsClearError(t *testing.T) {
	hub, osmo := newFakeLCD(t), newFakeLCD(t)
	tl := testTools(t, hub, osmo, Options{})
	api := newChainAPI(func(string) (routeindex.PathfinderChain, error) { return routeindex.PathfinderChain{Id: "x"}, nil })
	_, err := api.account(context.Background(), "x", "addr")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "no REST endpoints"))
	_ = tl
}

func TestEndpointFailover(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer dead.Close()
	live := newFakeLCD(t)
	live.json("/cosmos/auth/v1beta1/accounts/a", map[string]any{"account": map[string]any{"account_number": "3", "sequence": "4"}})
	api := newChainAPI(func(string) (routeindex.PathfinderChain, error) {
		return routeindex.PathfinderChain{Id: "x", Rests: []string{dead.URL, live.srv.URL}}, nil
	})
	acc, err := api.account(context.Background(), "x", "a")
	assert.NoError(t, err)
	assert.Equal(t, acc.AccountNumber, "3")
}

// A pruned or lagging node answers 404 for data other nodes still have. That
// must not be reported as "not found" while another endpoint can answer.
func TestNotFoundFallsThroughToNextEndpoint(t *testing.T) {
	pruned := newFakeLCD(t) // no handlers: 404 for everything
	full := newFakeLCD(t)
	full.json("/cosmos/tx/v1beta1/txs/ABCD", map[string]any{"tx_response": map[string]any{"txhash": "ABCD", "height": "7", "code": 0}})
	api := newChainAPI(func(string) (routeindex.PathfinderChain, error) {
		return routeindex.PathfinderChain{Id: "x", Rests: []string{pruned.srv.URL, full.srv.URL}}, nil
	})

	tx, err := api.tx(context.Background(), "x", "abcd")
	assert.NoError(t, err)
	assert.True(t, tx.Found)
	assert.Equal(t, tx.Height, "7")

	// Every endpoint says 404: only then is it genuinely not found.
	tx, err = api.tx(context.Background(), "x", "ffff")
	assert.NoError(t, err)
	assert.False(t, tx.Found)

	acc, err := api.account(context.Background(), "x", "unfunded")
	assert.NoError(t, err)
	assert.False(t, acc.Exists)
}

func TestProtoToolResultsExposeZeroValues(t *testing.T) {
	// found=false must be explicit, not an empty object.
	b, err := protoJSON(&v2beta.LookupDenomResponse{Found: false})
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(b), `"found":false`) || strings.Contains(string(b), `"found": false`))
}

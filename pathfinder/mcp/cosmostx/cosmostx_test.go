package cosmostx_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp/cosmostx"
	"github.com/zeebo/assert"
)

type goldenCase struct {
	Name         string `json:"name"`
	Mode         string `json:"mode"`
	Seq          uint64 `json:"seq"`
	Memo         string `json:"memo"`
	MsgValueB64  string `json:"msgValueB64"`
	BodyBytesB64 string `json:"bodyBytesB64"`
	AuthInfoB64  string `json:"authInfoB64"`
	SignDocB64   string `json:"signDocB64"`
	AminoSignDoc string `json:"aminoSignDoc"`
}

type golden struct {
	Pub   string       `json:"pub"`
	Cases []goldenCase `json:"cases"`
}

const contractMsg = `{"swap_and_action":{"user_swap":{"swap_exact_asset_in":{"swap_venue_name":"osmosis-poolmanager","operations":[{"pool":"1","denom_in":"uosmo","denom_out":"uatom"}]}},"min_asset":{"native":{"amount":"990","denom":"uatom"}},"timeout_timestamp":1760000000000000000,"post_swap_action":{"ibc_transfer":{"ibc_info":{"memo":"","receiver":"cosmos1r","recover_address":"osmo1rec","source_channel":"channel-0"}}},"affiliates":[]}}`

func goldenMsg(t *testing.T, name string) cosmostx.Msg {
	t.Helper()
	switch name {
	case "transfer":
		return &cosmostx.MsgTransfer{
			SourcePort: "transfer", SourceChannel: "channel-141",
			Token:  cosmostx.Coin{Denom: "uatom", Amount: "1000000"},
			Sender: "cosmos1sender", Receiver: "osmo1receiver",
			TimeoutTimestamp: 1760000000000000000,
			Memo:             `{"forward":{"receiver":"juno1x","port":"transfer","channel":"channel-5"}}`,
		}
	case "execute":
		return &cosmostx.MsgExecuteContract{
			Sender: "osmo1sender", Contract: "osmo1contract",
			Msg:   json.RawMessage(contractMsg),
			Funds: []cosmostx.Coin{{Denom: "uosmo", Amount: "1000"}},
		}
	}
	t.Fatalf("unknown golden case %s", name)
	return nil
}

func b64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	assert.NoError(t, err)
	return b
}

// The fixtures are produced by the CosmJS stack the client app signs with
// (testdata/cosmjs_golden.json), so these tests pin our encoders to it.
func TestMatchesCosmJS(t *testing.T) {
	raw, err := os.ReadFile("testdata/cosmjs_golden.json")
	assert.NoError(t, err)
	var g golden
	assert.NoError(t, json.Unmarshal(raw, &g))
	pub := b64(t, g.Pub)

	for _, c := range g.Cases {
		t.Run(c.Name+"/"+c.Mode, func(t *testing.T) {
			msg := goldenMsg(t, c.Name)
			assert.Equal(t, base64.StdEncoding.EncodeToString(msg.Marshal()), c.MsgValueB64)

			body := cosmostx.BuildTxBody([]cosmostx.Msg{msg}, c.Memo, 0)
			assert.Equal(t, base64.StdEncoding.EncodeToString(body), c.BodyBytesB64)

			fee := cosmostx.Fee{Amount: []cosmostx.Coin{{Denom: "uatom", Amount: "5000"}}, GasLimit: 200000}
			auth, err := cosmostx.BuildAuthInfo(cosmostx.Signer{PubKey: pub, Sequence: c.Seq, Mode: c.Mode}, fee)
			assert.NoError(t, err)
			assert.Equal(t, base64.StdEncoding.EncodeToString(auth), c.AuthInfoB64)

			doc := cosmostx.BuildSignDoc(body, auth, "cosmoshub-4", 99)
			assert.Equal(t, base64.StdEncoding.EncodeToString(doc), c.SignDocB64)

			_, canonical, err := cosmostx.AminoSignDoc([]cosmostx.Msg{msg}, c.Memo, "cosmoshub-4", 99, c.Seq, fee)
			assert.NoError(t, err)
			assert.Equal(t, string(canonical), c.AminoSignDoc)
		})
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	msgs := []cosmostx.Msg{
		goldenMsg(t, "transfer"),
		goldenMsg(t, "execute"),
		&cosmostx.MsgSwapExactAmountIn{
			Sender: "osmo1s", Routes: []cosmostx.SwapRoute{{PoolID: 1, TokenOutDenom: "uatom"}, {PoolID: 1135, TokenOutDenom: "uosmo"}},
			TokenIn: cosmostx.Coin{Denom: "uosmo", Amount: "5"}, TokenOutMinAmount: "4",
		},
		&cosmostx.MsgSplitRouteSwapExactAmountIn{
			Sender: "osmo1s", TokenInDenom: "uosmo", TokenOutMinAmount: "9",
			Routes: []cosmostx.SplitRoute{
				{Pools: []cosmostx.SwapRoute{{PoolID: 7, TokenOutDenom: "uatom"}}, TokenInAmount: "3"},
				{Pools: []cosmostx.SwapRoute{{PoolID: 8, TokenOutDenom: "uatom"}}, TokenInAmount: "2"},
			},
		},
	}
	body := cosmostx.BuildTxBody(msgs, "hello", 0)
	pub := make([]byte, 33)
	pub[0] = 2
	auth, err := cosmostx.BuildAuthInfo(cosmostx.Signer{PubKey: pub, Sequence: 3, Mode: cosmostx.SignModeAmino},
		cosmostx.Fee{Amount: []cosmostx.Coin{{Denom: "uosmo", Amount: "1"}}, GasLimit: 99})
	assert.NoError(t, err)
	tx := cosmostx.BuildTxRaw(body, auth, []byte{1, 2, 3})

	d, err := cosmostx.DecodeTx(tx)
	assert.NoError(t, err)
	assert.Equal(t, len(d.Messages), 4)
	assert.Equal(t, d.Memo, "hello")
	assert.Equal(t, d.Signatures, 1)
	assert.Equal(t, d.NonEmptySigs, 1)
	assert.Equal(t, d.Signers[0].SignMode, "amino")
	assert.Equal(t, d.Signers[0].Sequence, "3")
	assert.Equal(t, d.Fee.GasLimit, uint64(99))
	for i, m := range msgs {
		assert.True(t, d.Messages[i].Known)
		assert.Equal(t, d.Messages[i].TypeURL, m.TypeURL())
		assert.Equal(t, string(d.Messages[i].Msg.Marshal()), string(m.Marshal()))
	}
	assert.Equal(t, d.TxHash, cosmostx.TxHash(tx))
}

func TestUnknownMessageDecodesGenerically(t *testing.T) {
	body := cosmostx.BuildTxBody([]cosmostx.Msg{&cosmostx.RawMsg{URL: "/foo.v1.MsgBar", Value: []byte{0x0a, 0x01, 'x'}}}, "", 0)
	d, err := cosmostx.DecodeTx(cosmostx.BuildTxRaw(body, nil))
	assert.NoError(t, err)
	assert.False(t, d.Messages[0].Known)
	assert.Equal(t, d.Messages[0].TypeURL, "/foo.v1.MsgBar")
}

func TestDecodeRejectsGarbage(t *testing.T) {
	_, err := cosmostx.DecodeTx([]byte("not a transaction"))
	assert.Error(t, err)
}

func TestAminoSignDocRejectsMessagesWithoutAminoForm(t *testing.T) {
	_, _, err := cosmostx.AminoSignDoc([]cosmostx.Msg{&cosmostx.RawMsg{URL: "/x.Y"}}, "", "c", 1, 1, cosmostx.Fee{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "/x.Y"))
}

func TestBuildAuthInfoValidatesKeyAndMode(t *testing.T) {
	_, err := cosmostx.BuildAuthInfo(cosmostx.Signer{PubKey: []byte{1}}, cosmostx.Fee{})
	assert.Error(t, err)
	_, err = cosmostx.BuildAuthInfo(cosmostx.Signer{PubKey: make([]byte, 33), Mode: "textual"}, cosmostx.Fee{})
	assert.Error(t, err)
}

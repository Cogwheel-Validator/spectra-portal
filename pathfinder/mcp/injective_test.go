package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp/cosmostx"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	"github.com/zeebo/assert"
)

// The pubkey of private key 1 has the well-known Ethereum address
// 0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf. The same 20 bytes are its
// Injective address; the expected string was produced by CosmJS toBech32.
func TestEthAddressFromPubKeyKnownVector(t *testing.T) {
	var g struct {
		Address string `json:"address"`
	}
	raw, err := os.ReadFile("cosmostx/testdata/injective_golden.json")
	assert.NoError(t, err)
	assert.NoError(t, json.Unmarshal(raw, &g))

	addr, err := ethAddressFromPubKey("inj", validTestPubKey())
	assert.NoError(t, err)
	assert.Equal(t, addr, "inj10e0525sfrf53yh2aljmm3sn9jq5njk7lwfmzjf")
	assert.Equal(t, addr, g.Address)

	// The Cosmos derivation must give a different address for the same key.
	cosmosStyle, err := addressFromPubKey("inj", validTestPubKey())
	assert.NoError(t, err)
	assert.NotEqual(t, cosmosStyle, addr)
}

func injectiveFixture(t *testing.T, accountType string) (*tools, signDocInput) {
	t.Helper()
	lcd := newFakeLCD(t)
	chains := []routeindex.PathfinderChain{{
		Name: "Injective", Id: "injective-1", Bech32Prefix: "inj", Slip44: 60, AccountType: accountType,
		Rests: []string{lcd.srv.URL},
	}}
	pf := router.NewPathfinder(chains, routeindex.NewRouteIndex(), nil)
	tl := newTools(common.Deps{Pathfinder: pf}, Options{})

	pub := validTestPubKey()
	signer, err := ethAddressFromPubKey("inj", pub)
	assert.NoError(t, err)
	msg := &cosmostx.MsgTransfer{
		SourcePort: "transfer", SourceChannel: "channel-1", Token: cosmostx.Coin{Denom: "inj", Amount: "5"},
		Sender: signer, Receiver: testAddr(t, "cosmos", 1), TimeoutTimestamp: 42,
	}
	// Injective returns accounts as EthAccount wrapping base_account.
	lcd.json("/cosmos/auth/v1beta1/accounts/"+signer, map[string]any{"account": map[string]any{
		"@type":        "/injective.types.v1beta1.EthAccount",
		"base_account": map[string]any{"address": signer, "pub_key": nil, "account_number": "123", "sequence": "4"},
		"code_hash":    "0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
	}})
	lcd.json("/cosmos/base/node/v1beta1/config", map[string]any{"minimum_gas_price": "160000000inj"})
	lcd.json("/cosmos/tx/v1beta1/simulate", map[string]any{"gas_info": map[string]any{"gas_used": "100000"}})
	return tl, signDocInput{
		ChainID: "injective-1", SignerAddress: signer, PublicKey: base64.StdEncoding.EncodeToString(pub),
		Messages: []messageIn{{TypeURL: msg.TypeURL(), ValueBase64: base64.StdEncoding.EncodeToString(msg.Marshal())}},
	}
}

func TestBuildSignDocInjectiveDirect(t *testing.T) {
	tl, in := injectiveFixture(t, "ethermint")
	out, err := tl.buildSignDoc(context.Background(), in)
	assert.NoError(t, err)
	assert.Equal(t, out.AccountNumber, "123") // read through the EthAccount wrapper
	assert.Equal(t, out.Sequence, "4")
	assert.Equal(t, out.GasLimit, uint64(140000))
	assert.Equal(t, out.Fee.Amount[0].Denom, "inj")
	assert.Equal(t, out.Fee.Amount[0].Amount, "22400000000000") // 140000 * 160000000
	assert.NotEqual(t, out.SignDocBase64, "")

	body, _ := base64.StdEncoding.DecodeString(out.BodyBytesBase64)
	auth, _ := base64.StdEncoding.DecodeString(out.AuthInfoBytesBase64)
	assert.True(t, strings.Contains(string(auth), cosmostx.PubKeyTypeURLEthSecp256k1))
	assert.False(t, strings.Contains(string(auth), cosmostx.PubKeyTypeURLSecp256k1))

	// The assembled transaction still decodes (this is what assemble_signed_tx does).
	sig := make([]byte, 64)
	sig[0] = 1
	dec, err := cosmostx.DecodeTx(cosmostx.BuildTxRaw(body, auth, sig))
	assert.NoError(t, err)
	assert.Equal(t, dec.Signers[0].Sequence, "4")
	assert.Equal(t, dec.Signers[0].SignMode, "direct")
}

func TestBuildSignDocInjectiveRejectsCosmosStyleAddress(t *testing.T) {
	tl, in := injectiveFixture(t, "ethermint")
	// The sha256+ripemd160 address of the same key is NOT this key's Injective address.
	wrong, err := addressFromPubKey("inj", validTestPubKey())
	assert.NoError(t, err)
	in.SignerAddress = wrong
	_, err = tl.buildSignDoc(context.Background(), in)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "public key belongs to"))
}

func TestBuildSignDocInjectiveLedgerAminoIsRefused(t *testing.T) {
	tl, in := injectiveFixture(t, "ethermint")
	in.SignMode = "amino"
	_, err := tl.buildSignDoc(context.Background(), in)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "EIP-712"))
}

func TestBuildSignDocOtherAccountTypesStillRefused(t *testing.T) {
	tl, in := injectiveFixture(t, "ethereum")
	_, err := tl.buildSignDoc(context.Background(), in)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "does not support"))
}

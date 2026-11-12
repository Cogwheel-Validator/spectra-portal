package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp/cosmostx"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"golang.org/x/crypto/ripemd160" //nolint:staticcheck,gosec // deprecated but mandated by the Cosmos address format
	"golang.org/x/crypto/sha3"
)

const (
	maxMessages    = 20
	maxTxBytes     = 128 << 10
	gasSimMultiple = 14 // gas_limit = simulated gas * 1.4
)

// messageIn is a message exactly as build_transaction returned it.
type messageIn struct {
	TypeURL     string `json:"type_url"`
	ValueBase64 string `json:"value_base64" jsonschema:"copy value_base64 from build_transaction unchanged"`
}

type signDocInput struct {
	ChainID       string      `json:"chain_id"`
	SignerAddress string      `json:"signer_address" jsonschema:"bech32 address that signs; must be the sender of every message"`
	PublicKey     string      `json:"public_key_base64" jsonschema:"signer's 33-byte compressed secp256k1 public key, base64 (from the wallet)"`
	SignMode      string      `json:"sign_mode,omitempty" jsonschema:"direct (default; software wallets) or amino (Ledger / legacy)"`
	Messages      []messageIn `json:"messages" jsonschema:"the messages of ONE build_transaction step"`
	TxMemo        string      `json:"tx_memo,omitempty"`
	GasLimit      uint64      `json:"gas_limit,omitempty" jsonschema:"omit to simulate on-chain and use simulated gas x 1.4"`
	FeeDenom      string      `json:"fee_denom,omitempty"`
	FeeAmount     string      `json:"fee_amount,omitempty" jsonschema:"explicit fee; omit to derive from gas_price or the node's minimum gas price"`
	GasPrice      string      `json:"gas_price,omitempty" jsonschema:"decimal price per gas unit in fee_denom, e.g. 0.025"`
	AccountNumber *uint64     `json:"account_number,omitempty" jsonschema:"omit to look it up on-chain"`
	Sequence      *uint64     `json:"sequence,omitempty" jsonschema:"omit to look it up on-chain"`
}

// SignDocOut is everything a signer needs, in the encodings wallets accept.
type SignDocOut struct {
	ChainID       string       `json:"chain_id"`
	SignerAddress string       `json:"signer_address"`
	SignMode      string       `json:"sign_mode"`
	AccountNumber string       `json:"account_number"`
	Sequence      string       `json:"sequence"`
	GasLimit      uint64       `json:"gas_limit"`
	Fee           cosmostx.Fee `json:"fee"`
	TxMemo        string       `json:"tx_memo,omitempty"`
	Messages      []string     `json:"messages_summary"`

	BodyBytesBase64     string `json:"body_bytes_base64"`
	AuthInfoBytesBase64 string `json:"auth_info_bytes_base64"`

	// Direct mode.
	SignDocBase64 string         `json:"sign_doc_base64,omitempty"`
	SignDirect    map[string]any `json:"sign_direct,omitempty"`

	// Amino mode.
	AminoSignDoc          map[string]any `json:"amino_sign_doc,omitempty"`
	AminoSignDocCanonical string         `json:"amino_sign_doc_canonical,omitempty"`

	Notes []string `json:"notes"`
}

var gasPriceRe = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(.+)$`)

func (t *tools) buildSignDoc(ctx context.Context, in signDocInput) (*SignDocOut, error) {
	if len(in.Messages) == 0 || len(in.Messages) > maxMessages {
		return nil, fmt.Errorf("messages must contain between 1 and %d entries", maxMessages)
	}
	mode := in.SignMode
	if mode == "" {
		mode = cosmostx.SignModeDirect
	}
	ch, err := t.deps.Pathfinder.GetChainInfo(in.ChainID)
	if err != nil {
		return nil, fmt.Errorf("unknown chain %q", in.ChainID)
	}
	keyType := cosmostx.KeyTypeSecp256k1
	switch ch.AccountType {
	case "", "cosmos":
	case "ethermint":
		// Injective: eth_secp256k1 keys. Direct signing with a software wallet works; Ledger signs EIP-712 typed data, which is not built here.
		keyType = cosmostx.KeyTypeEthSecp256k1
		if mode == cosmostx.SignModeAmino {
			return nil, fmt.Errorf("chain %s uses Ethermint (eth_secp256k1) accounts: only sign_mode %q is supported. Ledger needs EIP-712 signing, which this server does not build; use a software wallet (e.g. Keplr) for this chain", in.ChainID, cosmostx.SignModeDirect)
		}
	default:
		return nil, fmt.Errorf("chain %s uses account type %q, which build_sign_doc does not support (only standard Cosmos and Ethermint/Injective accounts); sign this chain's transactions with a wallet that handles its own signing", in.ChainID, ch.AccountType)
	}
	pub, err := base64.StdEncoding.DecodeString(in.PublicKey)
	if err != nil || len(pub) != 33 {
		return nil, fmt.Errorf("public_key_base64 must be a base64 33-byte compressed secp256k1 key")
	}
	if _, err := secp256k1.ParsePubKey(pub); err != nil {
		return nil, fmt.Errorf("public_key_base64 is not a valid secp256k1 public key: %w", err)
	}
	if ch.Bech32Prefix != "" {
		want, err := addressFromKey(keyType, ch.Bech32Prefix, pub)
		if err != nil {
			return nil, err
		}
		if want != in.SignerAddress {
			return nil, fmt.Errorf("public key belongs to %s, not signer_address %s: wrong key or wrong chain", want, in.SignerAddress)
		}
	}

	msgs := make([]cosmostx.Msg, 0, len(in.Messages))
	summary := make([]string, 0, len(in.Messages))
	for _, m := range in.Messages {
		raw, err := base64.StdEncoding.DecodeString(m.ValueBase64)
		if err != nil {
			return nil, fmt.Errorf("message %s: value_base64 is not valid base64", m.TypeURL)
		}
		msg, err := cosmostx.ParseMsg(m.TypeURL, raw)
		if err != nil {
			return nil, err
		}
		if s, ok := messageSender(msg); ok && s != in.SignerAddress {
			return nil, fmt.Errorf("message %s has sender %s but signer_address is %s; a transaction can only be signed by its senders", m.TypeURL, s, in.SignerAddress)
		}
		msgs = append(msgs, msg)
		summary = append(summary, m.TypeURL)
	}

	out := &SignDocOut{ChainID: in.ChainID, SignerAddress: in.SignerAddress, SignMode: mode, TxMemo: in.TxMemo, Messages: summary}

	// Account number and sequence.
	var accNum, seq uint64
	if in.AccountNumber != nil && in.Sequence != nil {
		accNum, seq = *in.AccountNumber, *in.Sequence
	} else {
		acc, err := t.api.account(ctx, in.ChainID, in.SignerAddress)
		if err != nil {
			return nil, fmt.Errorf("could not look up the account: %w", err)
		}
		if !acc.Exists {
			return nil, fmt.Errorf("account %s does not exist on %s yet (it has never received funds); fund it first", in.SignerAddress, in.ChainID)
		}
		// Values come from a chain; garbage leaves them at 0.
		_, _ = fmt.Sscan(acc.AccountNumber, &accNum)
		_, _ = fmt.Sscan(acc.Sequence, &seq)
		if in.AccountNumber != nil {
			accNum = *in.AccountNumber
		}
		if in.Sequence != nil {
			seq = *in.Sequence
		}
	}
	out.AccountNumber, out.Sequence = u(accNum), u(seq)

	body := cosmostx.BuildTxBody(msgs, in.TxMemo, 0)
	signer := cosmostx.Signer{PubKey: pub, Sequence: seq, Mode: mode, KeyType: keyType}

	// Gas.
	gas := in.GasLimit
	if gas == 0 {
		simAuth, err := cosmostx.BuildAuthInfo(signer, cosmostx.Fee{GasLimit: 0})
		if err != nil {
			return nil, err
		}
		used, err := t.api.simulate(ctx, in.ChainID, cosmostx.BuildTxRaw(body, simAuth, make([]byte, 64)))
		if err != nil {
			return nil, fmt.Errorf("simulation failed (the transaction would probably fail on-chain): %w", err)
		}
		gas = used * gasSimMultiple / 10
		out.Notes = append(out.Notes, fmt.Sprintf("gas_limit %d = simulated %d x 1.4", gas, used))
	}
	out.GasLimit = gas

	// Fee.
	fee := cosmostx.Fee{GasLimit: gas, Amount: []cosmostx.Coin{}}
	switch {
	case in.FeeAmount != "":
		if in.FeeDenom == "" {
			return nil, fmt.Errorf("fee_denom is required with fee_amount")
		}
		fee.Amount = []cosmostx.Coin{{Denom: in.FeeDenom, Amount: in.FeeAmount}}
	default:
		denom, price, err := t.gasPrice(ctx, in)
		if err != nil {
			out.Notes = append(out.Notes, "no fee set: "+err.Error()+"; pass fee_denom + fee_amount (or gas_price) explicitly")
			break
		}
		amt := new(big.Rat).Mul(price, new(big.Rat).SetUint64(gas))
		n := new(big.Int).Div(new(big.Int).Add(amt.Num(), new(big.Int).Sub(amt.Denom(), big.NewInt(1))), amt.Denom()) // ceil
		fee.Amount = []cosmostx.Coin{{Denom: denom, Amount: n.String()}}
		out.Notes = append(out.Notes, fmt.Sprintf("fee %s%s = gas %d x price %s. If the node rejects it as insufficient, rebuild with a higher gas_price.", n, denom, gas, price.FloatString(6)))
	}
	out.Fee = fee

	auth, err := cosmostx.BuildAuthInfo(signer, fee)
	if err != nil {
		return nil, err
	}
	out.BodyBytesBase64 = base64.StdEncoding.EncodeToString(body)
	out.AuthInfoBytesBase64 = base64.StdEncoding.EncodeToString(auth)

	switch mode {
	case cosmostx.SignModeDirect:
		doc := cosmostx.BuildSignDoc(body, auth, in.ChainID, accNum)
		out.SignDocBase64 = base64.StdEncoding.EncodeToString(doc)
		out.SignDirect = map[string]any{
			"chain_id": in.ChainID, "account_number": u(accNum),
			"body_bytes_base64": out.BodyBytesBase64, "auth_info_bytes_base64": out.AuthInfoBytesBase64,
		}
		out.Notes = append(out.Notes,
			"Sign with the wallet's direct signer (e.g. Keplr signDirect / CosmJS signDirect) using sign_direct, or sign sha256(sign_doc_base64) with the key. The signature is 64 bytes (r||s).",
			"If the wallet lets the user change the fee, use the bodyBytes/authInfoBytes from the wallet's signed response (not these) when assembling.")
	case cosmostx.SignModeAmino:
		doc, canonical, err := cosmostx.AminoSignDoc(msgs, in.TxMemo, in.ChainID, accNum, seq, fee)
		if err != nil {
			return nil, err
		}
		out.AminoSignDoc = doc
		out.AminoSignDocCanonical = string(canonical)
		out.Notes = append(out.Notes,
			"Sign amino_sign_doc with the wallet's amino signer (e.g. Keplr signAmino / CosmJS signAmino); the signed bytes are amino_sign_doc_canonical. Required for Ledger.",
			"If the wallet changes the fee or memo in its signed response, call build_sign_doc again with those values so body/auth_info match what was signed.")
	default:
		return nil, fmt.Errorf("sign_mode must be %q or %q", cosmostx.SignModeDirect, cosmostx.SignModeAmino)
	}
	out.Notes = append(out.Notes, "Next: assemble_signed_tx with body_bytes_base64, auth_info_bytes_base64 and the wallet's signature.")
	return out, nil
}

func u(v uint64) string { return fmt.Sprint(v) }

// gasPrice resolves the fee denom and price per gas unit.
func (t *tools) gasPrice(ctx context.Context, in signDocInput) (denom string, price *big.Rat, err error) {
	if in.GasPrice != "" {
		if in.FeeDenom == "" {
			return "", nil, fmt.Errorf("fee_denom is required with gas_price")
		}
		p, ok := new(big.Rat).SetString(in.GasPrice)
		if !ok || p.Sign() < 0 {
			return "", nil, fmt.Errorf("invalid gas_price %q", in.GasPrice)
		}
		return in.FeeDenom, p, nil
	}
	var resp struct {
		MinimumGasPrice string `json:"minimum_gas_price"`
	}
	if err := t.api.do(ctx, in.ChainID, http.MethodGet, "/cosmos/base/node/v1beta1/config", nil, &resp); err != nil {
		return "", nil, fmt.Errorf("could not read the node's minimum gas price: %w", err)
	}
	for _, part := range strings.Split(resp.MinimumGasPrice, ",") {
		m := gasPriceRe.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			continue
		}
		if in.FeeDenom != "" && m[2] != in.FeeDenom {
			continue
		}
		p, _ := new(big.Rat).SetString(m[1])
		return m[2], p, nil
	}
	return "", nil, fmt.Errorf("the node reports no usable minimum gas price (%q)", resp.MinimumGasPrice)
}

func (c *chainAPI) simulate(ctx context.Context, chainID string, txBytes []byte) (uint64, error) {
	body, _ := json.Marshal(map[string]string{"tx_bytes": base64.StdEncoding.EncodeToString(txBytes)})
	var resp struct {
		GasInfo struct {
			GasUsed string `json:"gas_used"`
		} `json:"gas_info"`
	}
	if err := c.do(ctx, chainID, http.MethodPost, "/cosmos/tx/v1beta1/simulate", body, &resp); err != nil {
		return 0, err
	}
	var used uint64
	_, _ = fmt.Sscan(resp.GasInfo.GasUsed, &used) // garbage leaves 0, handled below
	if used == 0 {
		return 0, fmt.Errorf("simulation returned no gas usage")
	}
	return used, nil
}

// messageSender returns the sender field of the message types we know.
func messageSender(m cosmostx.Msg) (string, bool) {
	switch v := m.(type) {
	case *cosmostx.MsgTransfer:
		return v.Sender, true
	case *cosmostx.MsgExecuteContract:
		return v.Sender, true
	case *cosmostx.MsgSwapExactAmountIn:
		return v.Sender, true
	case *cosmostx.MsgSplitRouteSwapExactAmountIn:
		return v.Sender, true
	}
	return "", false
}

// addressFromKey derives the account address for a key type.
func addressFromKey(keyType, prefix string, pub []byte) (string, error) {
	if keyType == cosmostx.KeyTypeEthSecp256k1 {
		return ethAddressFromPubKey(prefix, pub)
	}
	return addressFromPubKey(prefix, pub)
}

// ethAddressFromPubKey derives the Ethermint / Injective address of a
// compressed secp256k1 key: the last 20 bytes of keccak256 over the
// uncompressed key without its 0x04 prefix, bech32-encoded. These are the same
// 20 bytes as the key's 0x... Ethereum address.
func ethAddressFromPubKey(prefix string, pub []byte) (string, error) {
	key, err := secp256k1.ParsePubKey(pub)
	if err != nil {
		return "", err
	}
	h := sha3.NewLegacyKeccak256()
	h.Write(key.SerializeUncompressed()[1:])
	conv, err := bech32.ConvertBits(h.Sum(nil)[12:], 8, 5, true)
	if err != nil {
		return "", err
	}
	return bech32.Encode(prefix, conv)
}

// addressFromPubKey derives the bech32 account address of a compressed
// secp256k1 key: RIPEMD160(SHA256(pubkey)).
func addressFromPubKey(prefix string, pub []byte) (string, error) {
	sha := sha256.Sum256(pub)
	h := ripemd160.New() //nolint:gosec // mandated by the Cosmos address format
	h.Write(sha[:])
	conv, err := bech32.ConvertBits(h.Sum(nil), 8, 5, true)
	if err != nil {
		return "", err
	}
	return bech32.Encode(prefix, conv)
}

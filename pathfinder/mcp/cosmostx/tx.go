package cosmostx

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Sign modes.
const (
	SignModeDirect = "direct"
	SignModeAmino  = "amino"

	signModeDirectEnum = 1
	signModeAminoEnum  = 127
)

// PubKeyTypeURLSecp256k1 is the Any type URL of a standard Cosmos secp256k1 key.
const PubKeyTypeURLSecp256k1 = "/cosmos.crypto.secp256k1.PubKey"

// Fee is the transaction fee.
type Fee struct {
	Amount   []Coin `json:"amount"`
	GasLimit uint64 `json:"gas_limit"`
}

// Signer describes the single signer of a transaction.
type Signer struct {
	// PubKey is the 33-byte compressed secp256k1 public key.
	PubKey   []byte
	Sequence uint64
	Mode     string // SignModeDirect or SignModeAmino
}

func signModeEnum(mode string) (uint64, error) {
	switch mode {
	case SignModeDirect, "":
		return signModeDirectEnum, nil
	case SignModeAmino:
		return signModeAminoEnum, nil
	}
	return 0, fmt.Errorf("unknown sign mode %q (use %q or %q)", mode, SignModeDirect, SignModeAmino)
}

// BuildTxBody encodes a cosmos.tx.v1beta1.TxBody.
func BuildTxBody(msgs []Msg, memo string, timeoutHeight uint64) []byte {
	var e encoder
	for _, m := range msgs {
		e.msg(1, marshalAny(m.TypeURL(), m.Marshal()))
	}
	e.str(2, memo)
	e.u64(3, timeoutHeight)
	return e.b
}

func marshalFee(f Fee) []byte {
	var e encoder
	for _, c := range f.Amount {
		e.msg(1, c.marshal())
	}
	e.u64(2, f.GasLimit)
	return e.b
}

// BuildAuthInfo encodes a cosmos.tx.v1beta1.AuthInfo for one secp256k1 signer.
func BuildAuthInfo(s Signer, f Fee) ([]byte, error) {
	if len(s.PubKey) != 33 {
		return nil, fmt.Errorf("public key must be a 33-byte compressed secp256k1 key, got %d bytes", len(s.PubKey))
	}
	mode, err := signModeEnum(s.Mode)
	if err != nil {
		return nil, err
	}
	var key, single, modeInfo, si, ai encoder
	key.bytes(1, s.PubKey)
	single.u64(1, mode)
	modeInfo.msg(1, single.b)
	si.msg(1, marshalAny(PubKeyTypeURLSecp256k1, key.b))
	si.msg(2, modeInfo.b)
	si.u64(3, s.Sequence)
	ai.msg(1, si.b)
	ai.msg(2, marshalFee(f))
	return ai.b, nil
}

// BuildSignDoc encodes a cosmos.tx.v1beta1.SignDoc (the bytes signed in
// direct mode).
func BuildSignDoc(bodyBytes, authInfoBytes []byte, chainID string, accountNumber uint64) []byte {
	var e encoder
	e.bytes(1, bodyBytes)
	e.bytes(2, authInfoBytes)
	e.str(3, chainID)
	e.u64(4, accountNumber)
	return e.b
}

// BuildTxRaw assembles a broadcastable cosmos.tx.v1beta1.TxRaw.
func BuildTxRaw(bodyBytes, authInfoBytes []byte, signatures ...[]byte) []byte {
	var e encoder
	e.bytes(1, bodyBytes)
	e.bytes(2, authInfoBytes)
	for _, s := range signatures {
		e.msg(3, s)
	}
	return e.b
}

// TxHash returns the upper-case hex SHA-256 of the raw transaction bytes,
// which is the transaction hash chains report.
func TxHash(txBytes []byte) string {
	sum := sha256.Sum256(txBytes)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// AminoSignDoc builds the legacy amino-JSON StdSignDoc as canonical JSON:
// keys sorted recursively, no insignificant whitespace. These are the exact
// bytes an amino signer signs. It fails if any message has no amino form.
func AminoSignDoc(msgs []Msg, memo, chainID string, accountNumber, sequence uint64, fee Fee) (doc map[string]any, canonical []byte, err error) {
	aminoMsgs := make([]*AminoMsg, 0, len(msgs))
	for _, m := range msgs {
		a, ok := m.Amino()
		if !ok {
			return nil, nil, fmt.Errorf("message %s has no amino-JSON form; use direct signing", m.TypeURL())
		}
		aminoMsgs = append(aminoMsgs, a)
	}
	feeAmount := fee.Amount
	if feeAmount == nil {
		feeAmount = []Coin{}
	}
	doc = map[string]any{
		"account_number": u64s(accountNumber),
		"chain_id":       chainID,
		"fee":            map[string]any{"amount": feeAmount, "gas": u64s(fee.GasLimit)},
		"memo":           memo,
		"msgs":           aminoMsgs,
		"sequence":       u64s(sequence),
	}
	canonical, err = CanonicalJSON(doc)
	return doc, canonical, err
}

// CanonicalJSON marshals v with keys sorted at every depth (including inside
// json.RawMessage values) and numbers preserved verbatim.
func CanonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	return json.Marshal(generic) // maps are emitted with sorted keys
}

// ---------------------------------------------------------------------------
// Decoding

// DecodedMsg is one message of a decoded transaction.
type DecodedMsg struct {
	TypeURL     string `json:"type_url"`
	ValueBase64 string `json:"value_base64"`
	Decoded     any    `json:"decoded,omitempty"`
	Amino       any    `json:"amino,omitempty"`
	Known       bool   `json:"known"`

	Msg Msg `json:"-"`
}

// DecodedSigner is one entry of auth_info.signer_infos.
type DecodedSigner struct {
	PubKeyType   string `json:"public_key_type,omitempty"`
	PubKeyBase64 string `json:"public_key_base64,omitempty"`
	SignMode     string `json:"sign_mode,omitempty"`
	Sequence     string `json:"sequence"`
}

// DecodedTx is a decoded Tx / TxRaw.
type DecodedTx struct {
	Messages      []DecodedMsg    `json:"messages"`
	Memo          string          `json:"memo,omitempty"`
	TimeoutHeight string          `json:"timeout_height,omitempty"`
	Signers       []DecodedSigner `json:"signer_infos"`
	Fee           *Fee            `json:"fee,omitempty"`
	Signatures    int             `json:"signature_count"`
	NonEmptySigs  int             `json:"non_empty_signature_count"`
	TxHash        string          `json:"tx_hash"`
}

// DecodeTx decodes a cosmos.tx.v1beta1.Tx or TxRaw (they share a wire layout
// for the fields we read).
func DecodeTx(txBytes []byte) (*DecodedTx, error) {
	top, err := parseFields(txBytes)
	if err != nil {
		return nil, fmt.Errorf("decode tx: %w", err)
	}
	out := &DecodedTx{TxHash: TxHash(txBytes)}
	var body, auth []byte
	for _, f := range top {
		switch f.num {
		case 1:
			body = f.b
		case 2:
			auth = f.b
		case 3:
			out.Signatures++
			if len(f.b) > 0 {
				out.NonEmptySigs++
			}
		}
	}
	if body == nil && auth == nil {
		return nil, fmt.Errorf("decode tx: no body or auth_info found; is this a Cosmos SDK Tx/TxRaw?")
	}
	if err := out.decodeBody(body); err != nil {
		return nil, err
	}
	if err := out.decodeAuthInfo(auth); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DecodedTx) decodeBody(body []byte) error {
	fs, err := parseFields(body)
	if err != nil {
		return fmt.Errorf("decode tx body: %w", err)
	}
	for _, f := range fs {
		switch f.num {
		case 1:
			url, val, err := parseAny(f.b)
			if err != nil {
				return fmt.Errorf("decode tx message: %w", err)
			}
			m, err := ParseMsg(url, val)
			if err != nil {
				return err
			}
			dm := DecodedMsg{
				TypeURL:     url,
				ValueBase64: base64.StdEncoding.EncodeToString(val),
				Msg:         m,
			}
			if _, raw := m.(*RawMsg); !raw {
				dm.Known = true
				dm.Decoded = m.ProtoJSON()
				if a, ok := m.Amino(); ok {
					dm.Amino = a
				}
			}
			d.Messages = append(d.Messages, dm)
		case 2:
			d.Memo = f.str()
		case 3:
			if f.u != 0 {
				d.TimeoutHeight = u64s(f.u)
			}
		}
	}
	return nil
}

func (d *DecodedTx) decodeAuthInfo(auth []byte) error {
	fs, err := parseFields(auth)
	if err != nil {
		return fmt.Errorf("decode auth_info: %w", err)
	}
	for _, f := range fs {
		switch f.num {
		case 1:
			s, err := decodeSignerInfo(f.b)
			if err != nil {
				return err
			}
			d.Signers = append(d.Signers, s)
		case 2:
			ff, err := parseFields(f.b)
			if err != nil {
				return fmt.Errorf("decode fee: %w", err)
			}
			fee := &Fee{Amount: []Coin{}}
			for _, x := range ff {
				switch x.num {
				case 1:
					c, err := parseCoin(x.b)
					if err != nil {
						return err
					}
					fee.Amount = append(fee.Amount, c)
				case 2:
					fee.GasLimit = x.u
				}
			}
			d.Fee = fee
		}
	}
	return nil
}

func decodeSignerInfo(b []byte) (DecodedSigner, error) {
	fs, err := parseFields(b)
	if err != nil {
		return DecodedSigner{}, fmt.Errorf("decode signer_info: %w", err)
	}
	var s DecodedSigner
	var seq uint64
	for _, f := range fs {
		switch f.num {
		case 1:
			url, val, err := parseAny(f.b)
			if err != nil {
				return s, err
			}
			s.PubKeyType = url
			if kf, err := parseFields(val); err == nil {
				for _, k := range kf {
					if k.num == 1 {
						s.PubKeyBase64 = base64.StdEncoding.EncodeToString(k.b)
					}
				}
			}
		case 2:
			mf, err := parseFields(f.b)
			if err != nil {
				return s, err
			}
			for _, m := range mf {
				if m.num != 1 {
					continue
				}
				sf, err := parseFields(m.b)
				if err != nil {
					return s, err
				}
				for _, x := range sf {
					if x.num == 1 {
						switch x.u {
						case signModeDirectEnum:
							s.SignMode = SignModeDirect
						case signModeAminoEnum:
							s.SignMode = SignModeAmino
						default:
							s.SignMode = fmt.Sprintf("mode_%d", x.u)
						}
					}
				}
			}
		case 3:
			seq = f.u
		}
	}
	s.Sequence = u64s(seq)
	return s, nil
}

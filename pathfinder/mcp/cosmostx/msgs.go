package cosmostx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
)

// Type URLs of the messages this package understands.
const (
	TypeURLMsgTransfer             = "/ibc.applications.transfer.v1.MsgTransfer"
	TypeURLMsgExecuteContract      = "/cosmwasm.wasm.v1.MsgExecuteContract"
	TypeURLMsgSwapExactAmountIn    = "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn"
	TypeURLMsgSplitRouteSwapAmount = "/osmosis.poolmanager.v1beta1.MsgSplitRouteSwapExactAmountIn"
)

// AminoMsg is a message in legacy amino-JSON form (what Ledger devices sign).
type AminoMsg struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// Msg is a Cosmos SDK message the codec can encode.
type Msg interface {
	// TypeURL is the protobuf Any type URL.
	TypeURL() string
	// Marshal returns the protobuf encoding of the message (the Any value).
	Marshal() []byte
	// ProtoJSON returns the proto3-JSON form with snake_case field names.
	ProtoJSON() any
	// Amino returns the amino-JSON form. ok is false when the message has no
	// amino representation this package knows.
	Amino() (msg *AminoMsg, ok bool)
}

// Coin is an amount of a denom.
type Coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

func (c Coin) marshal() []byte {
	var e encoder
	e.str(1, c.Denom)
	e.str(2, c.Amount)
	return e.b
}

func parseCoin(b []byte) (Coin, error) {
	fs, err := parseFields(b)
	if err != nil {
		return Coin{}, err
	}
	var c Coin
	for _, f := range fs {
		switch f.num {
		case 1:
			c.Denom = f.str()
		case 2:
			c.Amount = f.str()
		}
	}
	return c, nil
}

func u64s(v uint64) string { return strconv.FormatUint(v, 10) }

// ---------------------------------------------------------------------------
// MsgTransfer

// MsgTransfer is ibc.applications.transfer.v1.MsgTransfer.
type MsgTransfer struct {
	SourcePort            string `json:"source_port"`
	SourceChannel         string `json:"source_channel"`
	Token                 Coin   `json:"token"`
	Sender                string `json:"sender"`
	Receiver              string `json:"receiver"`
	TimeoutRevisionNumber uint64 `json:"timeout_revision_number,omitempty"`
	TimeoutRevisionHeight uint64 `json:"timeout_revision_height,omitempty"`
	// TimeoutTimestamp is in nanoseconds since the unix epoch.
	TimeoutTimestamp uint64 `json:"timeout_timestamp,omitempty"`
	Memo             string `json:"memo,omitempty"`
}

// TypeURL implements Msg.
func (m *MsgTransfer) TypeURL() string { return TypeURLMsgTransfer }

// Marshal implements Msg.
func (m *MsgTransfer) Marshal() []byte {
	var h encoder
	h.u64(1, m.TimeoutRevisionNumber)
	h.u64(2, m.TimeoutRevisionHeight)

	var e encoder
	e.str(1, m.SourcePort)
	e.str(2, m.SourceChannel)
	e.msg(3, m.Token.marshal())
	e.str(4, m.Sender)
	e.str(5, m.Receiver)
	e.msg(6, h.b)
	e.u64(7, m.TimeoutTimestamp)
	e.str(8, m.Memo)
	return e.b
}

// ProtoJSON implements Msg.
func (m *MsgTransfer) ProtoJSON() any {
	out := map[string]any{
		"source_port":    m.SourcePort,
		"source_channel": m.SourceChannel,
		"token":          m.Token,
		"sender":         m.Sender,
		"receiver":       m.Receiver,
		"timeout_height": map[string]string{
			"revision_number": u64s(m.TimeoutRevisionNumber),
			"revision_height": u64s(m.TimeoutRevisionHeight),
		},
		"timeout_timestamp": u64s(m.TimeoutTimestamp),
	}
	if m.Memo != "" {
		out["memo"] = m.Memo
	}
	return out
}

// Amino implements Msg. Zero-valued fields are omitted the way the SDK's
// x/tx aminojson encoder omits them, and timeout_height is always present.
func (m *MsgTransfer) Amino() (*AminoMsg, bool) {
	height := map[string]string{}
	if m.TimeoutRevisionNumber != 0 {
		height["revision_number"] = u64s(m.TimeoutRevisionNumber)
	}
	if m.TimeoutRevisionHeight != 0 {
		height["revision_height"] = u64s(m.TimeoutRevisionHeight)
	}
	v := map[string]any{
		"source_port":    m.SourcePort,
		"source_channel": m.SourceChannel,
		"token":          m.Token,
		"sender":         m.Sender,
		"receiver":       m.Receiver,
		"timeout_height": height,
	}
	if m.TimeoutTimestamp != 0 {
		v["timeout_timestamp"] = u64s(m.TimeoutTimestamp)
	}
	if m.Memo != "" {
		v["memo"] = m.Memo
	}
	return &AminoMsg{Type: "cosmos-sdk/MsgTransfer", Value: v}, true
}

func parseMsgTransfer(b []byte) (*MsgTransfer, error) {
	fs, err := parseFields(b)
	if err != nil {
		return nil, err
	}
	m := &MsgTransfer{}
	for _, f := range fs {
		switch f.num {
		case 1:
			m.SourcePort = f.str()
		case 2:
			m.SourceChannel = f.str()
		case 3:
			if m.Token, err = parseCoin(f.b); err != nil {
				return nil, err
			}
		case 4:
			m.Sender = f.str()
		case 5:
			m.Receiver = f.str()
		case 6:
			hf, err := parseFields(f.b)
			if err != nil {
				return nil, err
			}
			for _, h := range hf {
				switch h.num {
				case 1:
					m.TimeoutRevisionNumber = h.u
				case 2:
					m.TimeoutRevisionHeight = h.u
				}
			}
		case 7:
			m.TimeoutTimestamp = f.u
		case 8:
			m.Memo = f.str()
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// MsgExecuteContract

// MsgExecuteContract is cosmwasm.wasm.v1.MsgExecuteContract.
type MsgExecuteContract struct {
	Sender   string `json:"sender"`
	Contract string `json:"contract"`
	// Msg is the contract message as raw JSON.
	Msg   json.RawMessage `json:"msg"`
	Funds []Coin          `json:"funds"`
}

// TypeURL implements Msg.
func (m *MsgExecuteContract) TypeURL() string { return TypeURLMsgExecuteContract }

// Marshal implements Msg.
func (m *MsgExecuteContract) Marshal() []byte {
	var e encoder
	e.str(1, m.Sender)
	e.str(2, m.Contract)
	e.bytes(3, m.Msg)
	for _, c := range m.Funds {
		e.msg(5, c.marshal())
	}
	return e.b
}

func (m *MsgExecuteContract) funds() []Coin {
	if m.Funds == nil {
		return []Coin{}
	}
	return m.Funds
}

// ProtoJSON implements Msg.
func (m *MsgExecuteContract) ProtoJSON() any {
	return map[string]any{
		"sender":   m.Sender,
		"contract": m.Contract,
		"msg":      m.Msg,
		"funds":    m.funds(),
	}
}

// Amino implements Msg. The contract message is embedded as a JSON object
// (not base64), and funds is always present, as in wasmd's amino encoding.
func (m *MsgExecuteContract) Amino() (*AminoMsg, bool) {
	if !json.Valid(m.Msg) {
		return nil, false
	}
	return &AminoMsg{Type: "wasm/MsgExecuteContract", Value: map[string]any{
		"sender":   m.Sender,
		"contract": m.Contract,
		"msg":      m.Msg,
		"funds":    m.funds(),
	}}, true
}

func parseMsgExecuteContract(b []byte) (*MsgExecuteContract, error) {
	fs, err := parseFields(b)
	if err != nil {
		return nil, err
	}
	m := &MsgExecuteContract{}
	for _, f := range fs {
		switch f.num {
		case 1:
			m.Sender = f.str()
		case 2:
			m.Contract = f.str()
		case 3:
			m.Msg = json.RawMessage(append([]byte(nil), f.b...))
		case 5:
			c, err := parseCoin(f.b)
			if err != nil {
				return nil, err
			}
			m.Funds = append(m.Funds, c)
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Osmosis poolmanager swaps

// SwapRoute is osmosis.poolmanager.v1beta1.SwapAmountInRoute.
type SwapRoute struct {
	PoolID        uint64 `json:"pool_id"`
	TokenOutDenom string `json:"token_out_denom"`
}

func (r SwapRoute) marshal() []byte {
	var e encoder
	e.u64(1, r.PoolID)
	e.str(2, r.TokenOutDenom)
	return e.b
}

func parseSwapRoute(b []byte) (SwapRoute, error) {
	fs, err := parseFields(b)
	if err != nil {
		return SwapRoute{}, err
	}
	var r SwapRoute
	for _, f := range fs {
		switch f.num {
		case 1:
			r.PoolID = f.u
		case 2:
			r.TokenOutDenom = f.str()
		}
	}
	return r, nil
}

func (r SwapRoute) json() map[string]string {
	return map[string]string{"pool_id": u64s(r.PoolID), "token_out_denom": r.TokenOutDenom}
}

func routesJSON(rs []SwapRoute) []map[string]string {
	out := make([]map[string]string, len(rs))
	for i, r := range rs {
		out[i] = r.json()
	}
	return out
}

// MsgSwapExactAmountIn is osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn.
type MsgSwapExactAmountIn struct {
	Sender            string      `json:"sender"`
	Routes            []SwapRoute `json:"routes"`
	TokenIn           Coin        `json:"token_in"`
	TokenOutMinAmount string      `json:"token_out_min_amount"`
}

// TypeURL implements Msg.
func (m *MsgSwapExactAmountIn) TypeURL() string { return TypeURLMsgSwapExactAmountIn }

// Marshal implements Msg.
func (m *MsgSwapExactAmountIn) Marshal() []byte {
	var e encoder
	e.str(1, m.Sender)
	for _, r := range m.Routes {
		e.msg(2, r.marshal())
	}
	e.msg(3, m.TokenIn.marshal())
	e.str(4, m.TokenOutMinAmount)
	return e.b
}

func (m *MsgSwapExactAmountIn) value() map[string]any {
	return map[string]any{
		"sender":               m.Sender,
		"routes":               routesJSON(m.Routes),
		"token_in":             m.TokenIn,
		"token_out_min_amount": m.TokenOutMinAmount,
	}
}

// ProtoJSON implements Msg.
func (m *MsgSwapExactAmountIn) ProtoJSON() any { return m.value() }

// Amino implements Msg.
func (m *MsgSwapExactAmountIn) Amino() (*AminoMsg, bool) {
	return &AminoMsg{Type: "osmosis/poolmanager/swap-exact-amount-in", Value: m.value()}, true
}

func parseMsgSwapExactAmountIn(b []byte) (*MsgSwapExactAmountIn, error) {
	fs, err := parseFields(b)
	if err != nil {
		return nil, err
	}
	m := &MsgSwapExactAmountIn{}
	for _, f := range fs {
		switch f.num {
		case 1:
			m.Sender = f.str()
		case 2:
			r, err := parseSwapRoute(f.b)
			if err != nil {
				return nil, err
			}
			m.Routes = append(m.Routes, r)
		case 3:
			if m.TokenIn, err = parseCoin(f.b); err != nil {
				return nil, err
			}
		case 4:
			m.TokenOutMinAmount = f.str()
		}
	}
	return m, nil
}

// SplitRoute is osmosis.poolmanager.v1beta1.SwapAmountInSplitRoute.
type SplitRoute struct {
	Pools         []SwapRoute `json:"pools"`
	TokenInAmount string      `json:"token_in_amount"`
}

// MsgSplitRouteSwapExactAmountIn is
// osmosis.poolmanager.v1beta1.MsgSplitRouteSwapExactAmountIn.
type MsgSplitRouteSwapExactAmountIn struct {
	Sender            string       `json:"sender"`
	Routes            []SplitRoute `json:"routes"`
	TokenInDenom      string       `json:"token_in_denom"`
	TokenOutMinAmount string       `json:"token_out_min_amount"`
}

// TypeURL implements Msg.
func (m *MsgSplitRouteSwapExactAmountIn) TypeURL() string { return TypeURLMsgSplitRouteSwapAmount }

// Marshal implements Msg.
func (m *MsgSplitRouteSwapExactAmountIn) Marshal() []byte {
	var e encoder
	e.str(1, m.Sender)
	for _, r := range m.Routes {
		var re encoder
		for _, p := range r.Pools {
			re.msg(1, p.marshal())
		}
		re.str(2, r.TokenInAmount)
		e.msg(2, re.b)
	}
	e.str(3, m.TokenInDenom)
	e.str(4, m.TokenOutMinAmount)
	return e.b
}

func (m *MsgSplitRouteSwapExactAmountIn) value() map[string]any {
	routes := make([]map[string]any, len(m.Routes))
	for i, r := range m.Routes {
		routes[i] = map[string]any{"pools": routesJSON(r.Pools), "token_in_amount": r.TokenInAmount}
	}
	return map[string]any{
		"sender":               m.Sender,
		"routes":               routes,
		"token_in_denom":       m.TokenInDenom,
		"token_out_min_amount": m.TokenOutMinAmount,
	}
}

// ProtoJSON implements Msg.
func (m *MsgSplitRouteSwapExactAmountIn) ProtoJSON() any { return m.value() }

// Amino implements Msg.
func (m *MsgSplitRouteSwapExactAmountIn) Amino() (*AminoMsg, bool) {
	return &AminoMsg{Type: "osmosis/poolmanager/split-amount-in", Value: m.value()}, true
}

func parseMsgSplit(b []byte) (*MsgSplitRouteSwapExactAmountIn, error) {
	fs, err := parseFields(b)
	if err != nil {
		return nil, err
	}
	m := &MsgSplitRouteSwapExactAmountIn{}
	for _, f := range fs {
		switch f.num {
		case 1:
			m.Sender = f.str()
		case 2:
			rf, err := parseFields(f.b)
			if err != nil {
				return nil, err
			}
			var r SplitRoute
			for _, x := range rf {
				switch x.num {
				case 1:
					p, err := parseSwapRoute(x.b)
					if err != nil {
						return nil, err
					}
					r.Pools = append(r.Pools, p)
				case 2:
					r.TokenInAmount = x.str()
				}
			}
			m.Routes = append(m.Routes, r)
		case 3:
			m.TokenInDenom = f.str()
		case 4:
			m.TokenOutMinAmount = f.str()
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Unknown messages and parsing

// RawMsg is a message of a type this package does not understand.
type RawMsg struct {
	URL   string
	Value []byte
}

// TypeURL implements Msg.
func (m *RawMsg) TypeURL() string { return m.URL }

// Marshal implements Msg.
func (m *RawMsg) Marshal() []byte { return m.Value }

// ProtoJSON implements Msg.
func (m *RawMsg) ProtoJSON() any {
	return map[string]string{"value_base64": base64.StdEncoding.EncodeToString(m.Value)}
}

// Amino implements Msg.
func (m *RawMsg) Amino() (*AminoMsg, bool) { return nil, false }

// ParseMsg decodes an Any into a typed Msg, falling back to RawMsg for
// unknown type URLs. Known types that fail to parse return an error.
func ParseMsg(typeURL string, value []byte) (Msg, error) {
	var (
		m   Msg
		err error
	)
	switch typeURL {
	case TypeURLMsgTransfer:
		m, err = parseMsgTransfer(value)
	case TypeURLMsgExecuteContract:
		m, err = parseMsgExecuteContract(value)
	case TypeURLMsgSwapExactAmountIn:
		m, err = parseMsgSwapExactAmountIn(value)
	case TypeURLMsgSplitRouteSwapAmount:
		m, err = parseMsgSplit(value)
	default:
		return &RawMsg{URL: typeURL, Value: value}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", typeURL, err)
	}
	return m, nil
}

func marshalAny(typeURL string, value []byte) []byte {
	var e encoder
	e.str(1, typeURL)
	e.bytes(2, value)
	return e.b
}

func parseAny(b []byte) (typeURL string, value []byte, err error) {
	fs, err := parseFields(b)
	if err != nil {
		return "", nil, err
	}
	for _, f := range fs {
		switch f.num {
		case 1:
			typeURL = f.str()
		case 2:
			value = f.b
		}
	}
	return typeURL, value, nil
}

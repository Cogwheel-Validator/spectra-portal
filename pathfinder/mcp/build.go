package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp/cosmostx"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers/osmosis"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
)

const (
	defaultTimeoutMinutes = 10
	maxTimeoutMinutes     = 60
	defaultSlippageBps    = 100
)

// MessageOut is one unsigned message in every encoding a signer may need.
type MessageOut struct {
	TypeURL     string `json:"type_url"`
	ValueBase64 string `json:"value_base64"`
	// ProtoJSON is the proto3-JSON form (snake_case), handy for display and
	// for signers that take JSON.
	ProtoJSON any `json:"proto_json"`
	// Amino is the legacy amino-JSON form used by Ledger and amino signers.
	Amino *cosmostx.AminoMsg `json:"amino,omitempty"`
}

func messageOut(m cosmostx.Msg) MessageOut {
	out := MessageOut{
		TypeURL:     m.TypeURL(),
		ValueBase64: base64.StdEncoding.EncodeToString(m.Marshal()),
		ProtoJSON:   m.ProtoJSON(),
	}
	if a, ok := m.Amino(); ok {
		out.Amino = a
	}
	return out
}

// TxStep is one transaction the user must sign and broadcast.
type TxStep struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Description string `json:"description"`
	// ChainID is the chain this transaction is signed for and sent to.
	ChainID string `json:"chain_id"`
	// Signer is the address that signs (the message sender).
	Signer string `json:"signer"`
	// WaitFor, when set, says this step must only be sent after the previous
	// step's funds have arrived (use track_transfer to confirm).
	WaitFor  string       `json:"wait_for,omitempty"`
	Messages []MessageOut `json:"messages"`
	// SuggestedMemo is an optional tx-level memo (distinct from the IBC memo).
	SuggestedMemo string `json:"suggested_tx_memo,omitempty"`
	// SuggestedGasLimit is a conservative default; build_sign_doc can simulate
	// for a tighter value.
	SuggestedGasLimit uint64 `json:"suggested_gas_limit"`
	// AmountFromSwap marks a transfer that moves the output of the earlier
	// swap step. It is built with the swap's guaranteed minimum output; once
	// the swap is done, set its amount to the actual output (track_transfer's
	// swap_output) with set_transfer_amount before signing.
	AmountFromSwap bool `json:"amount_from_swap,omitempty"`
}

// BuildResult is the output of build_transaction.
type BuildResult struct {
	Success bool     `json:"success"`
	Error   string   `json:"error,omitempty"`
	Mode    string   `json:"mode"`
	Route   any      `json:"route,omitempty"`
	Steps   []TxStep `json:"steps,omitempty"`
	// Warnings are things the user must be told before signing.
	Warnings []string `json:"warnings,omitempty"`
	Next     string   `json:"next,omitempty"`
}

type buildInput struct {
	ChainFrom      string         `json:"chain_from" jsonschema:"source chain ID"`
	TokenFrom      string         `json:"token_from" jsonschema:"denom, IBC denom, or symbol@origin_chain"`
	AmountIn       string         `json:"amount_in" jsonschema:"amount in base units (e.g. 1000000 for 1 ATOM)"`
	ChainTo        string         `json:"chain_to" jsonschema:"destination chain ID"`
	TokenTo        string         `json:"token_to,omitempty" jsonschema:"destination token; empty keeps the same token (no swap)"`
	Addresses      []addressInput `json:"addresses" jsonschema:"the user's address on EVERY chain of the route (run find_route without addresses to see required_chains)"`
	Mode           string         `json:"mode,omitempty" jsonschema:"smart (default): fewest transactions using packet-forward / ibc-hooks. manual: one transaction per leg, no middleware"`
	SlippageBps    uint32         `json:"slippage_bps,omitempty" jsonschema:"max swap slippage in basis points (default 100 = 1%)"`
	TimeoutMinutes int            `json:"timeout_minutes,omitempty" jsonschema:"IBC packet timeout in minutes (default 10, max 60)"`
}

// builder turns a route into unsigned transaction steps. It mirrors the step
// logic of the Spectra client app so agents and the UI behave identically.
type builder struct {
	addr    map[string]string
	timeout uint64 // absolute IBC timeout, ns since epoch
	steps   []TxStep
	warn    []string
}

func (b *builder) transfer(id, typ, desc string, leg *models.IBCLeg, receiver, memo string, wait string) {
	msg := &cosmostx.MsgTransfer{
		SourcePort: leg.Port, SourceChannel: leg.Channel,
		Token:  cosmostx.Coin{Denom: leg.Token.ChainDenom, Amount: leg.Amount},
		Sender: b.addr[leg.FromChain], Receiver: receiver,
		TimeoutTimestamp: b.timeout, Memo: memo,
	}
	// for now this is a conservative default
	gas := uint64(300_000)
	if memo != "" {
		gas = 600_000
	}
	b.steps = append(b.steps, TxStep{
		ID: id, Type: typ, Description: desc, ChainID: leg.FromChain, Signer: msg.Sender,
		WaitFor: wait, Messages: []MessageOut{messageOut(msg)},
		SuggestedMemo: "Spectra IBC transfer", SuggestedGasLimit: gas,
	})
}

func legDesc(l *models.IBCLeg) string {
	return fmt.Sprintf("IBC transfer %s %s from %s to %s via %s/%s", l.Amount, l.Token.ChainDenom, l.FromChain, l.ToChain, l.Port, l.Channel)
}

func (b *builder) manualLegs(prefix string, legs []*models.IBCLeg, firstWait string) {
	for i, l := range legs {
		wait := ""
		if i > 0 {
			wait = fmt.Sprintf("the %s arrives on %s from the previous step", l.Token.ChainDenom, l.FromChain)
		} else if firstWait != "" {
			wait = firstWait
		}
		b.transfer(fmt.Sprintf("%s-%d", prefix, i+1), "ibc_transfer", legDesc(l), l, b.addr[l.ToChain], "", wait)
	}
}

func (b *builder) direct(r *models.DirectRoute) {
	b.transfer("direct", "ibc_transfer", legDesc(r.Transfer), r.Transfer, b.addr[r.Transfer.ToChain], "", "")
}

func (b *builder) indirect(r *models.IndirectRoute, smart bool) {
	if smart && r.SupportsPFM && r.PFMMemo != "" {
		l := r.Legs[0]
		b.transfer("pfm", "ibc_transfer_pfm",
			fmt.Sprintf("Single transaction: send %s %s from %s; packet-forward-middleware relays it through %v to the destination", l.Amount, l.Token.ChainDenom, l.FromChain, r.Path[1:]),
			l, b.addr[l.ToChain], r.PFMMemo, "")
		return
	}
	b.manualLegs("leg", r.Legs, "")
}

func (b *builder) broker(r *models.BrokerRoute, smart bool, slippage uint32) error {
	ex := r.Execution
	switch {
	case smart && ex != nil && ex.UsesWasm && ex.SmartContractData != nil && ex.SmartContractData.Wasm != nil && len(r.InboundLegs) == 0:
		w := ex.SmartContractData.Wasm
		raw, err := json.Marshal(w.Msg)
		if err != nil {
			return fmt.Errorf("encode contract message: %w", err)
		}
		var funds []cosmostx.Coin
		if r.Swap != nil && r.Swap.TokenIn != nil && r.Swap.AmountIn != "" {
			funds = []cosmostx.Coin{{Denom: r.Swap.TokenIn.ChainDenom, Amount: r.Swap.AmountIn}}
		}
		chain := r.Path[0]
		msg := &cosmostx.MsgExecuteContract{Sender: b.addr[chain], Contract: w.Contract, Msg: raw, Funds: funds}
		b.steps = append(b.steps, TxStep{
			ID: "wasm-swap", Type: "wasm_swap_and_forward", ChainID: chain, Signer: msg.Sender,
			Description:   fmt.Sprintf("Single transaction on %s: swap %s then forward to %s through the ibc-hooks entry-point contract", chain, swapDesc(r), r.Path[len(r.Path)-1]),
			Messages:      []MessageOut{messageOut(msg)},
			SuggestedMemo: "Spectra WASM execution", SuggestedGasLimit: 1_200_000,
		})
	case smart && ex != nil && ex.Memo != nil && *ex.Memo != "" && len(r.InboundLegs) > 0 && ex.IBCReceiver != nil:
		l := r.InboundLegs[0]
		b.transfer("swap-memo", "ibc_transfer_with_swap_memo",
			fmt.Sprintf("Single transaction: send %s %s from %s; the memo makes %s swap %s and forward the result to %s", l.Amount, l.Token.ChainDenom, l.FromChain, l.ToChain, swapDesc(r), r.Path[len(r.Path)-1]),
			l, *ex.IBCReceiver, *ex.Memo, "")
		b.warn = append(b.warn,
			"This is a swap executed by a smart contract on the broker chain after the IBC transfer lands. If the swap fails or slips past the minimum, funds are refunded to the recover address on the broker chain: "+deref(ex.RecoverAddress)+".")
	default:
		return b.brokerManual(r, slippage)
	}
	return nil
}

func swapDesc(r *models.BrokerRoute) string {
	if r.Swap == nil || r.Swap.TokenIn == nil || r.Swap.TokenOut == nil {
		return "tokens"
	}
	return fmt.Sprintf("%s %s for ~%s %s", r.Swap.AmountIn, r.Swap.TokenIn.ChainDenom, r.Swap.AmountOut, r.Swap.TokenOut.ChainDenom)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// brokerManual builds inbound transfers, the on-chain swap, then outbound transfers.
func (b *builder) brokerManual(r *models.BrokerRoute, slippage uint32) error {
	b.manualLegs("inbound", r.InboundLegs, "")
	minOut := ""
	if r.Swap != nil {
		swapMsg, min, err := b.swapMessage(r, slippage)
		if err != nil {
			return err
		}
		minOut = min
		chain := r.Path[len(r.InboundLegs)]
		wait := ""
		if len(r.InboundLegs) > 0 {
			wait = fmt.Sprintf("the %s arrives on %s from the previous step", r.Swap.TokenIn.ChainDenom, chain)
		}
		b.steps = append(b.steps, TxStep{
			ID: "swap", Type: "osmosis_swap", ChainID: chain, Signer: b.addr[chain], WaitFor: wait,
			Description:   "Swap " + swapDesc(r) + " on " + chain,
			Messages:      []MessageOut{messageOut(swapMsg)},
			SuggestedMemo: "Spectra Osmosis Swap", SuggestedGasLimit: 800_000,
		})
	}
	for i, l := range r.OutboundLegs {
		wait := fmt.Sprintf("the swap output (%s) is on %s", l.Token.ChainDenom, l.FromChain)
		if i > 0 {
			wait = fmt.Sprintf("the %s arrives on %s from the previous step", l.Token.ChainDenom, l.FromChain)
		}
		if minOut == "" {
			b.transfer(fmt.Sprintf("outbound-%d", i+1), "ibc_transfer", legDesc(l), l, b.addr[l.ToChain], "", wait)
			continue
		}
		// The swap may return less than quoted (down to minOut). A transfer of
		// the quoted amount would then fail, so start from the minimum and let
		// the signer raise it to the actual output.
		leg := *l
		leg.Amount = minOut
		b.transfer(fmt.Sprintf("outbound-%d", i+1), "ibc_transfer", legDesc(&leg)+" (the swap's minimum output; set to the actual output before signing)", &leg, b.addr[l.ToChain], "", wait)
		b.steps[len(b.steps)-1].AmountFromSwap = true
	}
	if minOut != "" && len(r.OutboundLegs) > 0 {
		b.warn = append(b.warn,
			"The outbound transfers carry the swap's minimum output ("+minOut+"). After the swap, read swap_output from track_transfer and call set_transfer_amount on each step with amount_from_swap before signing it, or the difference stays on "+r.OutboundLegs[0].FromChain+".")
	}
	return nil
}

// swapMessage returns the swap message and its minimum output.
func (b *builder) swapMessage(r *models.BrokerRoute, slippage uint32) (cosmostx.Msg, string, error) {
	var rd *osmosis.RouteData
	switch v := r.Swap.RouteData.(type) {
	case *osmosis.RouteData:
		rd = v
	case osmosis.RouteData:
		rd = &v
	}
	if rd == nil || len(rd.Routes) == 0 {
		return nil, "", fmt.Errorf("the swap has no Osmosis route data; use mode=smart")
	}
	chain := b.addr[r.Path[len(r.InboundLegs)]]
	minOut, err := applySlippage(r.Swap.AmountOut, slippage)
	if err != nil {
		return nil, "", err
	}
	for _, rt := range rd.Routes {
		for _, p := range rt.Pools {
			if p.ID < 0 {
				return nil, "", fmt.Errorf("the swap route has an invalid pool id %d", p.ID)
			}
		}
	}
	pools := func(rt osmosis.Route) []cosmostx.SwapRoute {
		out := make([]cosmostx.SwapRoute, len(rt.Pools))
		for i, p := range rt.Pools {
			id := uint64(p.ID) //nolint:gosec // checked non-negative above
			out[i] = cosmostx.SwapRoute{PoolID: id, TokenOutDenom: p.TokenOutDenom}
		}
		return out
	}
	if len(rd.Routes) > 1 {
		m := &cosmostx.MsgSplitRouteSwapExactAmountIn{Sender: chain, TokenInDenom: r.Swap.TokenIn.ChainDenom, TokenOutMinAmount: minOut}
		for _, rt := range rd.Routes {
			m.Routes = append(m.Routes, cosmostx.SplitRoute{Pools: pools(rt), TokenInAmount: rt.InAmount})
		}
		return m, minOut, nil
	}
	return &cosmostx.MsgSwapExactAmountIn{
		Sender: chain, Routes: pools(rd.Routes[0]),
		TokenIn:           cosmostx.Coin{Denom: r.Swap.TokenIn.ChainDenom, Amount: r.Swap.AmountIn},
		TokenOutMinAmount: minOut,
	}, minOut, nil
}

type setAmountInput struct {
	Message MessageIn `json:"message" jsonschema:"the MsgTransfer of a build_transaction step (type_url + value_base64)"`
	Amount  string    `json:"amount" jsonschema:"new amount in base units, e.g. the swap_output amount from track_transfer"`
}

// MessageIn is a message as passed back in by a caller.
type MessageIn struct {
	TypeURL     string `json:"type_url"`
	ValueBase64 string `json:"value_base64"`
}

// setTransferAmount re-encodes an IBC MsgTransfer with a new amount; nothing
// else in the message changes.
func setTransferAmount(in setAmountInput) (*MessageOut, error) {
	if in.Message.TypeURL != cosmostx.TypeURLMsgTransfer {
		return nil, fmt.Errorf("only %s messages can be changed", cosmostx.TypeURLMsgTransfer)
	}
	if err := common.ValidateAmount(in.Amount); err != nil {
		return nil, fmt.Errorf("amount must be a positive integer in base units")
	}
	raw, err := base64.StdEncoding.DecodeString(in.Message.ValueBase64)
	if err != nil {
		return nil, fmt.Errorf("value_base64 is not valid base64")
	}
	m, err := cosmostx.ParseMsg(in.Message.TypeURL, raw)
	if err != nil {
		return nil, err
	}
	tr, ok := m.(*cosmostx.MsgTransfer)
	if !ok {
		return nil, fmt.Errorf("message did not decode as %s", cosmostx.TypeURLMsgTransfer)
	}
	tr.Token.Amount = in.Amount
	out := messageOut(tr)
	return &out, nil
}

// applySlippage returns amount * (10000-bps) / 10000, rounded down.
func applySlippage(amount string, bps uint32) (string, error) {
	n, ok := new(big.Int).SetString(amount, 10)
	if !ok || n.Sign() < 0 {
		return "", fmt.Errorf("invalid swap output amount %q", amount)
	}
	if bps > 10000 {
		return "", fmt.Errorf("slippage_bps must be at most 10000")
	}
	n.Mul(n, big.NewInt(int64(10000-bps)))
	n.Quo(n, big.NewInt(10000))
	return n.String(), nil
}

func (t *tools) build(_ context.Context, in buildInput) (*BuildResult, error) {
	mode := in.Mode
	if mode == "" {
		mode = "smart"
	}
	if mode != "smart" && mode != "manual" {
		return nil, fmt.Errorf("mode must be %q or %q", "smart", "manual")
	}
	if len(in.Addresses) == 0 {
		return nil, fmt.Errorf("addresses are required; call find_route without addresses first to see required_chains")
	}
	tm := in.TimeoutMinutes
	if tm <= 0 {
		tm = defaultTimeoutMinutes
	}
	if tm > maxTimeoutMinutes {
		return nil, fmt.Errorf("timeout_minutes must be at most %d", maxTimeoutMinutes)
	}
	slippage := in.SlippageBps
	if slippage == 0 {
		slippage = defaultSlippageBps
	}

	req := &v2beta.FindPathRequest{
		ChainFrom: in.ChainFrom, TokenFromDenom: in.TokenFrom, AmountIn: in.AmountIn,
		ChainTo: in.ChainTo, TokenToDenom: in.TokenTo,
		SmartRoute: mode == "smart", SlippageBps: slippage,
	}
	if err := common.ValidateAmount(in.AmountIn); err != nil {
		return nil, err
	}
	b := &builder{addr: map[string]string{}, timeout: uint64(t.now().Add(time.Duration(tm) * time.Minute).UnixNano())}
	for _, a := range in.Addresses {
		req.Addresses = append(req.Addresses, &v2beta.ChainAddress{ChainId: a.ChainID, Address: a.Address})
		b.addr[a.ChainID] = a.Address
	}

	route, err := t.findPath.FindRoute(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", connect.CodeOf(err), errMessage(err))
	}
	res := &BuildResult{Mode: mode, Success: route.Success}
	if !route.Success {
		res.Error = route.ErrorMessage
		return res, nil
	}

	smart := mode == "smart"
	switch {
	case route.Direct != nil:
		b.direct(route.Direct)
		res.Route = route.Direct
	case route.Indirect != nil:
		b.indirect(route.Indirect, smart)
		res.Route = route.Indirect
	case route.BrokerSwap != nil:
		if err := b.broker(route.BrokerSwap, smart, slippage); err != nil {
			return nil, err
		}
		res.Route = route.BrokerSwap
	default:
		return nil, fmt.Errorf("the pathfinder returned a route type this builder does not support: %s", route.RouteType)
	}
	res.Steps = b.steps
	res.Warnings = append(slices.Clone(b.warn),
		"These are UNSIGNED messages. Show the user the amounts, chains, recipient and steps and get explicit approval before anyone signs.",
		fmt.Sprintf("IBC timeout is %d minutes from now; build again if signing is delayed past it.", tm))
	res.Next = "For each step in order: call build_sign_doc, have the user's wallet sign it, call assemble_signed_tx, then broadcast_signed_tx (if enabled) or broadcast it yourself, and confirm with track_transfer before starting a step that has wait_for."
	return res, nil
}

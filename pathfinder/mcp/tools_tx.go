package mcp

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp/cosmostx"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type accountInput struct {
	ChainID string `json:"chain_id"`
	Address string `json:"address"`
}

type assembleInput struct {
	BodyBytes     string `json:"body_bytes_base64"`
	AuthInfoBytes string `json:"auth_info_bytes_base64"`
	Signature     string `json:"signature_base64" jsonschema:"the wallet's 64-byte (r||s) signature, base64"`
}

type broadcastInput struct {
	ChainID string `json:"chain_id"`
	TxBytes string `json:"tx_bytes_base64" jsonschema:"signed transaction from assemble_signed_tx"`
}

type trackInput struct {
	ChainID string `json:"chain_id" jsonschema:"chain the transaction was sent on"`
	TxHash  string `json:"tx_hash"`
	Follow  *bool  `json:"follow,omitempty" jsonschema:"follow forwarded packets across intermediate chains (default true)"`
}

type getTxInput struct {
	ChainID       string `json:"chain_id"`
	TxHash        string `json:"tx_hash"`
	IncludeEvents bool   `json:"include_events,omitempty"`
}

func (t *tools) registerTx(s *sdk.Server) {
	readOnly := &sdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(true)}

	sdk.AddTool(s, &sdk.Tool{
		Name: "build_transaction",
		Description: "Build the UNSIGNED transaction steps for a transfer or swap route. Needs the user's address on every chain of the route. " +
			"Each step lists chain_id, signer and messages (type_url + value_base64 + proto_json + amino). Steps are ordered; a step with wait_for must wait until the previous step's funds arrive. " +
			"Nothing is signed or sent. Present the plan to the user and get approval before continuing to build_sign_doc.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in buildInput) (*sdk.CallToolResult, any, error) {
		res, err := t.build(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(res)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_account",
		Description: "Get an address's on-chain account number, sequence and balances (via the chain's REST endpoints).",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in accountInput) (*sdk.CallToolResult, any, error) {
		acc, err := t.api.account(ctx, in.ChainID, in.Address)
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{"account": acc}
		if bal, err := t.api.balances(ctx, in.ChainID, in.Address); err == nil {
			out["balances"] = bal
		} else {
			out["balances_error"] = err.Error()
		}
		return jsonResult(out)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name: "build_sign_doc",
		Description: "Encode one build_transaction step into the exact bytes a wallet signs: direct SignDoc or amino-JSON sign doc. " +
			"Looks up account number/sequence, simulates gas and derives the fee unless you provide them. Verifies signer_address matches the public key and every message sender. " +
			"Pass messages exactly as returned by build_transaction; do not re-encode them yourself.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in signDocInput) (*sdk.CallToolResult, any, error) {
		out, err := t.buildSignDoc(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(out)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "assemble_signed_tx",
		Description: "Combine body_bytes, auth_info_bytes and the wallet's signature into broadcastable transaction bytes (base64) and its tx hash. Pure encoding; submits nothing.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *sdk.CallToolRequest, in assembleInput) (*sdk.CallToolResult, any, error) {
		body, err := base64.StdEncoding.DecodeString(in.BodyBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("body_bytes_base64 is not valid base64")
		}
		auth, err := base64.StdEncoding.DecodeString(in.AuthInfoBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("auth_info_bytes_base64 is not valid base64")
		}
		sig, err := base64.StdEncoding.DecodeString(in.Signature)
		if err != nil {
			return nil, nil, fmt.Errorf("signature_base64 is not valid base64")
		}
		if len(sig) != 64 {
			return nil, nil, fmt.Errorf("signature must be 64 bytes (r||s), got %d; DER signatures must be converted first", len(sig))
		}
		tx := cosmostx.BuildTxRaw(body, auth, sig)
		dec, err := cosmostx.DecodeTx(tx)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]any{
			"tx_bytes_base64": base64.StdEncoding.EncodeToString(tx),
			"tx_hash":         dec.TxHash,
			"decoded":         dec,
		})
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_tx",
		Description: "Fetch a transaction by hash from a chain: inclusion height, success/failure code and log.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in getTxInput) (*sdk.CallToolResult, any, error) {
		r, err := t.api.tx(ctx, in.ChainID, in.TxHash)
		if err != nil {
			return nil, nil, err
		}
		if in.IncludeEvents {
			return jsonResult(map[string]any{"tx": r, "events": r.Events})
		}
		return jsonResult(r)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name: "track_transfer",
		Description: "Track an IBC transfer from its source transaction: confirms inclusion, then follows each IBC packet to its destination (including packets forwarded through intermediate chains) and reports in_progress / completed / failed / timed_out with per-hop detail. " +
			"Poll every ~30s until status is not in_progress.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in trackInput) (*sdk.CallToolResult, any, error) {
		follow := in.Follow == nil || *in.Follow
		res, err := t.trackerFn.track(ctx, in.ChainID, in.TxHash, follow)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(res)
	})

	if t.opts.Broadcast {
		sdk.AddTool(s, &sdk.Tool{
			Name: "broadcast_signed_tx",
			Description: "Submit an ALREADY SIGNED transaction to the chain. Only Spectra route messages are accepted (IBC transfers, Osmosis swaps, and wasm calls to the configured swap entry-point contract). " +
				"This moves real funds: only call it after the user approved the exact transaction.",
			Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(true)},
		}, func(ctx context.Context, _ *sdk.CallToolRequest, in broadcastInput) (*sdk.CallToolResult, any, error) {
			out, err := t.broadcast(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return jsonResult(out)
		})
	}
}

func ptr[T any](v T) *T { return &v }

// broadcast validates a signed transaction against the Spectra allowlist and submits it.
func (t *tools) broadcast(ctx context.Context, in broadcastInput) (map[string]any, error) {
	ch, err := t.deps.Pathfinder.GetChainInfo(in.ChainID)
	if err != nil {
		return nil, fmt.Errorf("unknown chain %q", in.ChainID)
	}
	raw, err := base64.StdEncoding.DecodeString(in.TxBytes)
	if err != nil {
		return nil, fmt.Errorf("tx_bytes_base64 is not valid base64")
	}
	if len(raw) == 0 || len(raw) > maxTxBytes {
		return nil, fmt.Errorf("transaction must be 1..%d bytes", maxTxBytes)
	}
	dec, err := cosmostx.DecodeTx(raw)
	if err != nil {
		return nil, err
	}
	if len(dec.Messages) == 0 || len(dec.Messages) > maxMessages {
		return nil, fmt.Errorf("transaction must contain 1..%d messages", maxMessages)
	}
	if dec.NonEmptySigs == 0 || dec.NonEmptySigs != len(dec.Signers) {
		return nil, fmt.Errorf("transaction is not fully signed (%d signatures for %d signers)", dec.NonEmptySigs, len(dec.Signers))
	}
	for _, m := range dec.Messages {
		switch msg := m.Msg.(type) {
		case *cosmostx.MsgTransfer, *cosmostx.MsgSwapExactAmountIn, *cosmostx.MsgSplitRouteSwapExactAmountIn:
		case *cosmostx.MsgExecuteContract:
			if ch.IBCHooksContract == "" || msg.Contract != ch.IBCHooksContract {
				return nil, fmt.Errorf("MsgExecuteContract to %s is not allowed: only the configured swap entry-point contract on %s can be called", msg.Contract, in.ChainID)
			}
		default:
			return nil, fmt.Errorf("message type %s is not allowed through this endpoint; broadcast it with your own node", m.TypeURL)
		}
	}
	res, err := t.api.broadcast(ctx, in.ChainID, raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"tx_hash": dec.TxHash, "accepted": res.Code == 0, "result": res}
	if res.Code == 0 {
		out["next"] = "Accepted into the mempool, not yet in a block. Call track_transfer with this tx_hash and chain_id."
	} else {
		out["next"] = "The node rejected the transaction (see result.raw_log). Nothing was sent. Fix the cause (fee, sequence, balance) and rebuild with build_sign_doc."
	}
	return out, nil
}

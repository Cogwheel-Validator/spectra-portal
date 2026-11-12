package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp/cosmostx"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/addressing"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
	"github.com/btcsuite/btcutil/bech32"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type decodeTxInput struct {
	TxBase64 string `json:"tx_base64,omitempty" jsonschema:"transaction bytes (Tx or TxRaw), base64"`
	TxHex    string `json:"tx_hex,omitempty" jsonschema:"transaction bytes, hex (alternative to tx_base64)"`
}

type addressInfoInput struct {
	Address string `json:"address"`
}

type convertAddressInput struct {
	Address       string `json:"address"`
	TargetChainID string `json:"target_chain_id"`
}

type ibcDenomInput struct {
	BaseDenom string `json:"base_denom" jsonschema:"denom on the origin chain, e.g. uatom"`
	TracePath string `json:"trace_path" jsonschema:"port/channel hops as seen on the receiving chain, newest first, e.g. transfer/channel-141 or transfer/channel-0/transfer/channel-5"`
}

func (t *tools) registerUtil(s *sdk.Server) {
	readOnly := &sdk.ToolAnnotations{ReadOnlyHint: true}

	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_chain_endpoints",
		Description: "Get a chain's connection details: healthy RPC and REST endpoints, bech32 prefix, SLIP-44, account type, SDK version, packet-forward and swap-contract support. Use these to query or broadcast yourself.",
		Annotations: readOnly,
	}, func(_ context.Context, _ *sdk.CallToolRequest, in chainInput) (*sdk.CallToolResult, any, error) {
		ch, err := t.deps.Pathfinder.GetChainInfo(in.ChainID)
		if err != nil {
			return nil, nil, fmt.Errorf("unknown chain %q", in.ChainID)
		}
		return jsonResult(map[string]any{
			"chain_id": ch.Id, "name": ch.Name, "bech32_prefix": ch.Bech32Prefix, "slip44": ch.Slip44,
			"account_type": ch.AccountType, "cosmos_sdk_version": ch.CosmosSdkVersion,
			"supports_packet_forward_middleware": ch.HasPFM, "is_swap_broker": ch.Broker,
			"swap_entrypoint_contract": ch.IBCHooksContract,
			"rpc_endpoints":            ch.RPCs, "rest_endpoints": ch.Rests,
			"endpoints_note": endpointsNote(ch),
		})
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "decode_tx",
		Description: "Decode a transaction (base64 or hex of Tx/TxRaw) into messages, memo, fee, signers and signature count. Known Spectra message types are decoded to JSON; others are returned as type_url + base64. Use it to inspect what a wallet is about to sign or what was broadcast.",
		Annotations: readOnly,
	}, func(_ context.Context, _ *sdk.CallToolRequest, in decodeTxInput) (*sdk.CallToolResult, any, error) {
		var raw []byte
		var err error
		switch {
		case in.TxBase64 != "":
			raw, err = base64.StdEncoding.DecodeString(in.TxBase64)
		case in.TxHex != "":
			raw, err = hex.DecodeString(strings.TrimPrefix(in.TxHex, "0x"))
		default:
			return nil, nil, fmt.Errorf("provide tx_base64 or tx_hex")
		}
		if err != nil {
			return nil, nil, fmt.Errorf("could not decode input: %w", err)
		}
		if len(raw) > maxTxBytes {
			return nil, nil, fmt.Errorf("transaction larger than %d bytes", maxTxBytes)
		}
		dec, err := cosmostx.DecodeTx(raw)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(dec)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "address_info",
		Description: "Decode a bech32 address: prefix, 20/32-byte payload in hex, checksum validity, and which supported chains use that prefix.",
		Annotations: readOnly,
	}, func(_ context.Context, _ *sdk.CallToolRequest, in addressInfoInput) (*sdk.CallToolResult, any, error) {
		prefix, data5, err := bech32.Decode(in.Address)
		if err != nil {
			return jsonResult(map[string]any{"valid": false, "error": err.Error()})
		}
		data, err := bech32.ConvertBits(data5, 5, 8, false)
		if err != nil {
			return jsonResult(map[string]any{"valid": false, "error": err.Error()})
		}
		var chains []string
		for _, id := range t.deps.Pathfinder.GetAllChains() {
			if ch, err := t.deps.Pathfinder.GetChainInfo(id); err == nil && ch.Bech32Prefix == prefix {
				chains = append(chains, id)
			}
		}
		return jsonResult(map[string]any{"valid": true, "prefix": prefix, "bytes": len(data), "hex": hex.EncodeToString(data), "chains_using_prefix": chains})
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "convert_address",
		Description: "Re-encode an address for another chain's bech32 prefix. Only valid when both chains share the same SLIP-44 coin type (same key derivation); otherwise it errors instead of returning a wrong address.",
		Annotations: readOnly,
	}, func(_ context.Context, _ *sdk.CallToolRequest, in convertAddressInput) (*sdk.CallToolResult, any, error) {
		conv := addressing.NewAddressConverter(t.allChains())
		out, err := conv.ConvertAddress(in.Address, in.TargetChainID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]string{"address": out, "chain_id": in.TargetChainID})
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "compute_ibc_denom",
		Description: "Compute the ibc/<HASH> denom a token has after crossing IBC: ibc/ + uppercase SHA-256 of '<trace_path>/<base_denom>'.",
		Annotations: readOnly,
	}, func(_ context.Context, _ *sdk.CallToolRequest, in ibcDenomInput) (*sdk.CallToolResult, any, error) {
		path := strings.Trim(in.TracePath, "/")
		if path == "" || in.BaseDenom == "" {
			return nil, nil, fmt.Errorf("base_denom and trace_path are required")
		}
		parts := strings.Split(path, "/")
		if len(parts)%2 != 0 {
			return nil, nil, fmt.Errorf("trace_path must be port/channel pairs, e.g. transfer/channel-0")
		}
		sum := sha256.Sum256([]byte(path + "/" + in.BaseDenom))
		return jsonResult(map[string]string{"denom": "ibc/" + strings.ToUpper(hex.EncodeToString(sum[:])), "trace": path + "/" + in.BaseDenom})
	})
}

func (t *tools) allChains() []routeindex.PathfinderChain {
	var out []routeindex.PathfinderChain
	for _, id := range t.deps.Pathfinder.GetAllChains() {
		if ch, err := t.deps.Pathfinder.GetChainInfo(id); err == nil {
			out = append(out, ch)
		}
	}
	return out
}

func endpointsNote(ch routeindex.PathfinderChain) string {
	if len(ch.RPCs) == 0 && len(ch.Rests) == 0 {
		return "No endpoints in this Pathfinder's config. Regenerate it with the config manager, or use the chain registry."
	}
	return "Endpoints were health-checked when the config was generated; they can go stale, so fall back to another if one fails."
}

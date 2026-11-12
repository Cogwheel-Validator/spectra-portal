# Spectra Pathfinder MCP

The Pathfinder can expose a [Model Context Protocol](https://modelcontextprotocol.io)
endpoint so that AI agents
can **plan, build and track** cross-chain (IBC) transfers and DEX swaps. It never
holds keys and never signs: the user's own wallet signs. See [MCP_SIGNING.md](MCP_SIGNING.md)
for why, and [PATHFINDER_SIGN.md](PATHFINDER_SIGN.md) for the local program that
connects the user's Keplr.

Contents:

- [What it is for](#what-it-is-for)
- [Enable it](#enable-it-operators)
- [Connect a client](#connect-a-client)
- [Typical conversation](#a-typical-conversation)
- [Tools](#tools)
- [Configuration](#configuration)
- [Safety](#safety-model)
- [Operating it](#operating-notes)
- [Limits](#limits-and-what-is-untested)

## What it is for

| Good for | Not for |
| --- | --- |
| "Which route is cheapest for ATOM → OSMO?" Route finding, token and denom lookup, quotes | Replacing the dApp for a person who just wants to click through a transfer |
| Building unsigned transactions an agent can explain and a wallet can sign | Holding funds or keys for a user |
| Following a transfer across chains (including packet-forward hops) until it completes, fails or times out | A Ledger on Injective (needs EIP-712 signing) and other non-standard account types |
| Bots, scripts and other agents that need a programmatic interface with no UI | |

## Enable it (operators)

Off by default. In `rpc-config.toml` (or the matching `PATHFINDER_*` environment
variables):

```toml
enable_mcp = true                  # serve /mcp (streamable HTTP)
mcp_public_url = "https://pathfinder.example.com"   # recommended; see below
enable_mcp_broadcast = false       # broadcast_signed_tx; usually unnecessary (see below)
enable_mcp_inline_ui = false       # experimental inline signer view
```

- `mcp_public_url`: the externally reachable base URL, **without** `/mcp`.
  `prepare_transfer` puts it into the `pathfinder-sign` command. If empty, the
  server derives it from the request's `Host` / `X-Forwarded-*` headers, which is
  only reliable if your proxy sets them. **Set it behind a proxy.**
- `enable_mcp_broadcast`: lets MCP clients submit already-signed transactions .
  through the server, restricted to IBC transfers, Osmosis swaps and calls to
  the chain's configured swap entry-point contract. `pathfinder-sign` does not need
  it, because Keplr broadcasts through its own nodes.
- `enable_mcp_inline_ui`: serves the signer as an MCP App. Experimental!
  Most chat hosts will not let a wallet extension into it. Leave it off unless
  you are testing.
- Chain REST/RPC endpoints, `account_type` and `cosmos_sdk_version` come from
  the generated Pathfinder config. The account, simulate, track and broadcast
  tools need a config generated after those fields were added; regenerate it with
  the config manager if they report a missing endpoint.

## Connect a client

Any client that supports MCP over streamable HTTP can use `https://<host>/mcp`.

```bash
# Claude Code
claude mcp add --transport http spectra https://pathfinder.example.com/mcp
```

```json
// Generic client configuration
{"mcpServers": {"spectra": {"type": "http", "url": "https://pathfinder.example.com/mcp"}}}
```

For clients that only support local stdio servers, use a bridge such as `mcp-remote`. The agent should call
`get_execution_guide` first (also served as the resource `spectra://guide/execution`); the server instructions say so.

To let the agent sign through your browser wallet, install [pathfinder-sign](PATHFINDER_SIGN.md) on the machine where the
browser with Keplr runs.

## A typical conversation

> **You:** Send 5 ATOM from Cosmos Hub to Osmosis.
>
> **Agent** (calls `find_route`, `lookup_denom`): Route: 5 ATOM, `cosmoshub-4` → `osmosis-1`, direct IBC transfer, to your
> own address on Osmosis. This is a quote that goes stale. Do you want to proceed?
>
> **You:** Yes.
>
> **Agent** (calls `prepare_transfer`, runs `pathfinder-sign ...`, or gives you the command): Your browser is opening a
> page. Connect Keplr, check the amounts and recipient, and approve.
>
> *(You approve in Keplr; the program prints `{"status":"completed","transactions":[...]}`.)*
>
> **Agent** (calls `track_transfer`): Sent on Cosmos Hub, received on Osmosis. Hash `9F2C...`.

## Tools

All tools are read-only except `broadcast_signed_tx`. None of them signs.

### Planning

| Tool | What it does |
| --- | --- |
| `get_execution_guide` | **Read first.** Safety rules and the procedure (same text as the `spectra://guide/execution` resource) |
| `list_chains`, `get_chain_info`, `get_chain_tokens`, `lookup_denom`, `get_token_denoms` | Chains, tokens, denom resolution (same data as the query RPC) |
| `get_chain_endpoints` | Healthy RPC/REST endpoints, prefix, SLIP-44, SDK version, swap contract |
| `find_route` | Route preview (no addresses; lists `required_chains`) or a route with execution data |

### Signing

| Tool | What it does |
| --- | --- |
| `prepare_transfer` | **Preferred.** Validates a transfer and returns the ready `pathfinder-sign` command plus the signing request as structured data. Needs no addresses: the page reads them from Keplr |
| `get_signing_options` | The ways to sign, what each needs, and where to download `pathfinder-sign` |

### Building (manual flow)

| Tool | What it does |
| --- | --- |
| `build_transaction` | Ordered **unsigned** steps (`smart` = fewest transactions via packet-forward / ibc-hooks, `manual` = one per leg). Messages in `type_url` + `value_base64`, proto JSON and amino JSON. Needs an address per chain |
| `build_sign_doc` | Direct SignDoc or amino sign doc for one step. Looks up account number/sequence, simulates gas, derives the fee, and checks that the public key matches the signer and every message sender |
| `set_transfer_amount` | Re-encodes a step's MsgTransfer with a new amount. Used on manual-mode steps marked `amount_from_swap`, which are built with the swap's minimum output: after the swap, set them to its actual output (`swap_output` from `track_transfer`) |
| `assemble_signed_tx` | body + auth_info + the wallet's 64-byte signature → broadcastable bytes and hash |
| `broadcast_signed_tx` | **Only with `enable_mcp_broadcast`.** Submits a signed tx; rejects unsigned txs and anything outside the allowlist |

### Tracking and helpers

| Tool | What it does |
| --- | --- |
| `track_transfer` | Follows each IBC packet (including forwarded hops) and reports `in_progress` / `completed` / `failed` / `timed_out` with per-hop detail. A hop counts as done only once its receiving transaction was found, so forwarded packets are never missed. For a swap it also returns `swap_output` |
| `get_account`, `get_tx` | Account number / sequence / balances; a transaction's result |
| `decode_tx`, `address_info`, `convert_address`, `compute_ibc_denom` | Decode and encode helpers |

### `prepare_transfer` in detail

Input: `chain_from`, `token_from`, `amount_in` (base units), `chain_to`, and optionally `token_to`, `recipient`, `mode`,
`slippage_bps`. It:

1. Builds the signing request, using `mcp_public_url` (or headers) as the Pathfinder URL.
2. **Validates** every value. Chain IDs, denoms and addresses may only contain letters, digits and `/ _ . : @ -`, because
   they end up in a command line. Shell metacharacters are rejected.
3. Runs the route search now, so a missing route or a bad token is reported immediately.
4. Returns text for the agent with the command and the next steps, and structured data:
   `command`, `signing_request` (`{pathfinder_url, request}`), `request`, `preview`.

`recipient` should be set only when the user explicitly gave an address. Otherwise funds go to the user's own address on
the destination chain, taken from Keplr. If the agent sets it, the signing page shows a warning.

## Configuration

| Key | Env | Default | Meaning |
| --- | --- | --- | --- |
| `enable_mcp` | `PATHFINDER_ENABLE_MCP` | `false` | Serve `/mcp` |
| `mcp_public_url` | `PATHFINDER_MCP_PUBLIC_URL` | empty | Public base URL used in signing commands |
| `enable_mcp_broadcast` | `PATHFINDER_ENABLE_MCP_BROADCAST` | `false` | Add `broadcast_signed_tx` |
| `enable_mcp_inline_ui` | `PATHFINDER_ENABLE_MCP_INLINE_UI` | `false` | Experimental inline signer view |

`/mcp` shares the server's rate limit and concurrency throttle. See `rpc-config.example.toml`.

## Safety model

- **No keys on the server, ever.** Pathfinder builds unsigned transactions; wallets sign.
- **The wallet is the gate.** The signing page decodes what will be signed and the user approves in Keplr.
- **Agent input is untrusted.** Values that reach a command line are restricted to a safe character set; token names,
  memos and chain metadata are treated as data by the guide's rules.
- **Broadcast is opt-in** and allowlisted (IBC transfer, Osmosis swaps, swap contract calls only).
- **The guide tells the agent** never to ask for a key or mnemonic, to show the plan and get a "yes" before signing,
  to verify recipients, and to run `pathfinder-sign` only with the exact command returned.
- **Request logs** show tool and argument names, never values (tested).

## Operating notes

- **Reverse proxy:** MCP uses long-lived streams. Disable response buffering for `/mcp` and allow long timeouts.
- **Public URL:** set `mcp_public_url`, and serve over https (`pathfinder-sign` accepts http only for localhost).
- **Binaries:** `pathfinder-sign` is published with each release as `pathfinder-sign-<version>-<os>-<arch>` plus a
  checksums file. Point users to the releases page; `get_signing_options` does it for agents.
- **Upgrades:** the `build_transaction` step logic mirrors the client app's `generateStepsFromResponse` and task
  provider logic. If either changes, change both.

## Injective

Injective accounts are Ethermint accounts: `eth_secp256k1` keys, `inj1...` addresses derived with keccak256 (the same
20 bytes as the key's `0x...` address), and an `EthAccount` account type. `build_sign_doc` handles them when the chain's
`account_type` is `ethermint`:

- The address check uses the keccak derivation, so a key is only accepted for its own `inj1...` address.
- The `AuthInfo` carries `/injective.crypto.v1beta1.ethsecp256k1.PubKey`. Its bytes are tested against the client app's own
  encoder (`pathfinder/mcp/cosmostx/testdata/injective_golden.json`).
- The account number and sequence are read through the `EthAccount` wrapper; the fee uses the node's minimum gas price
  (Injective's is large, for example `160000000inj`, so fees are big integers).
- Only `sign_mode: direct` works. Keplr does the keccak step itself when signing, so the signer page needs no extra code.
- **Not supported: a Ledger on Injective.** It signs EIP-712 typed data instead of an amino document. The client app has
  that logic (`client_app/src/lib/injective/eip712.ts`); it is not ported to the server. `build_sign_doc` with
  `sign_mode: amino` on an Ethermint chain returns an explanatory error.
- Swaps are Osmosis-only, so on Injective only IBC transfers apply.

This path is tested against fixtures and a fake chain API, not a live Injective node or Keplr: try a small transfer first.

## Limits and what is untested

- Account types: standard secp256k1 Cosmos accounts and **Injective** (Ethermint, `account_type = "ethermint"`) with a software wallet in direct mode. A **Ledger on Injective is not supported** (it signs EIP-712 typed data, which the server does not build; see below). Plain `ethereum` accounts are not supported.
- Ack-success detection goes no further than the receiving chain's `write_acknowledgement` event.
- The encoders (`pathfinder/mcp/cosmostx`) are verified byte-for-byte against CosmJS fixtures. Osmosis swap field numbers
  were checked against the client app's ts-proto code, not golden bytes.
- **`pathfinder-sign` with a real Keplr has not been exercised end to end yet.** The server, proxy, validation and the MCP
  client are covered by tests (`go test ./signer/... ./pathfinder/mcp/...`) but the in-page Keplr calls
  (`signDirect`, `signAmino`, `sendTx`) have not been run against a live wallet. Try a small transfer first.
- The inline view is experimental and was not verified against the final MCP Apps specification (see MCP_SIGNING.md).
- Live checks against real chains are opt-in: `go test -tags live ./pathfinder/mcp/` (see `live_test.go`).

# Pathfinder v2beta API

v2beta is the current, recommended API surface. It's split into two services so route-finding and static lookups can evolve independently without forcing a version bump on the other:

- **`pathfinder.v2beta.FindPathService`** - `FindPath` (unary) and `FindPathStreaming` (bidirectional streaming).
- **`pathfinder.v2beta.PathfinderQueryService`** - `LookupDenom`, `GetTokenDenoms`, `GetChainInfo`, `ListSupportedChains`, `GetChainTokens`.

See [`overview.md`](./overview.md) for the protocol/transport details shared with v1 and the accepted denom syntax. If you're coming from v1, see [`v1.md`](./v1.md) for what changed.

Machine-readable OpenAPI 3.1 specs for both services (merged into one file) are generated at [`../openapi/v2beta.openapi.yaml`](../openapi/v2beta.openapi.yaml) (`make generate-openapi`). Note that `FindPathStreaming` doesn't map cleanly onto a REST-shaped OpenAPI operation - the state machine described below is the authoritative reference for it, not the generated spec.

## The `ChainAddress` / `addresses` model

v1's `FindPathRequest` takes a single `sender_address` and `receiver_address`. If the route the Pathfinder finds needs an address on an intermediate chain (a broker chain for a swap, or a PFM hop), v1 derives that address automatically from the sender/receiver pair via SLIP-44-guarded bech32 re-encoding. That works for the common case, but different chains can use different SLIP-44 coin types, so a blindly re-derived address can be malformed or simply wrong for some chains.

v2beta replaces `sender_address`/`receiver_address` with `repeated ChainAddress addresses` - a list of `{chain_id, address}` pairs, one per chain the route may touch. You resolve and supply the correct address for every chain yourself; nothing is derived on your behalf.

To make the contrast concrete, here's the same 3-chain broker-swap route (ATOM on Cosmos Hub -> swap on Osmosis -> USDC on Noble) addressed both ways:

**v1** - two fields, Osmosis address derived server-side:

```json
{
  "chain_from": "cosmoshub-4",
  "token_from_denom": "uatom",
  "amount_in": "5000000",
  "chain_to": "noble-1",
  "token_to_denom": "uusdc",
  "sender_address": "cosmos1zjqm4lngspfqkp68psuv4suxwjfxftkeg9fmqp",
  "receiver_address": "noble1zjqm4lngspfqkp68psuv4suxwjfxftkenx9k4n"
}
```

**v2beta** - one address per chain touched, nothing derived:

```json
{
  "chain_from": "cosmoshub-4",
  "token_from_denom": "uatom",
  "amount_in": "5000000",
  "chain_to": "noble-1",
  "token_to_denom": "uusdc",
  "addresses": [
    { "chain_id": "cosmoshub-4", "address": "cosmos1zjqm4lngspfqkp68psuv4suxwjfxftkeg9fmqp" },
    { "chain_id": "osmosis-1", "address": "osmo1zjqm4lngspfqkp68psuv4suxwjfxftken7rwm0" },
    { "chain_id": "noble-1", "address": "noble1zjqm4lngspfqkp68psuv4suxwjfxftkenx9k4n" }
  ]
}
```

`addresses` must either be empty (see mock/discovery mode below) or contain at least 2 entries; a non-empty list that's missing an address for a chain the found route actually needs is rejected with an error naming the missing chain IDs.

## Mock / discovery mode

On the unary `FindPath` call, you can leave `addresses` empty. The Pathfinder still computes a route (using generated placeholder addresses internally), but the response is marked with `response_code: RESPONSE_CODE_MOCK_ADDRESSES`, omits `execution`/memo data (since there's no real address to build a transaction against), and populates `required_chains` with every chain ID that would need a real address if you were to submit the same request again for real.

This is intended for a UI that wants to show a user what a route looks like - how many hops, which chains and tokens are involved - before every wallet has been connected. Once the user is ready to actually execute, re-issue the same request with a full `addresses` list to get a real, executable response (`response_code: RESPONSE_CODE_OK`).

Example mock-mode response for the same Cosmos Hub -> Osmosis -> Noble route:

```json
{
  "success": true,
  "error_message": "",
  "response_code": "RESPONSE_CODE_MOCK_ADDRESSES",
  "required_chains": ["cosmoshub-4", "osmosis-1", "noble-1"],
  "broker_swap": {
    "path": ["cosmoshub-4", "osmosis-1", "noble-1"],
    "inbound_legs": [
      {
        "from_chain": "cosmoshub-4",
        "to_chain": "osmosis-1",
        "channel": "channel-141",
        "port": "transfer",
        "token": {
          "chain_denom": "uatom",
          "base_denom": "uatom",
          "origin_chain": "cosmoshub-4",
          "is_native": true
        },
        "amount": "5000000"
      }
    ],
    "swap": {
      "broker": "osmosis-sqs",
      "token_in": {
        "chain_denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
        "base_denom": "uatom",
        "origin_chain": "cosmoshub-4",
        "is_native": false
      },
      "token_out": {
        "chain_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
        "base_denom": "uusdc",
        "origin_chain": "noble-1",
        "is_native": false
      },
      "amount_in": "5000000",
      "amount_out": "9680597",
      "price_impact": "-0.002367475333974154",
      "effective_fee": "0.001500000000000000"
    },
    "outbound_legs": [
      {
        "from_chain": "osmosis-1",
        "to_chain": "noble-1",
        "channel": "channel-750",
        "port": "transfer",
        "token": {
          "chain_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
          "base_denom": "uusdc",
          "origin_chain": "noble-1",
          "is_native": false
        },
        "amount": "9680597"
      }
    ],
    "outbound_supports_pfm": true
  }
}
```

Note there's no `execution` field at all in mock mode - only the route shape, amounts, and `required_chains` are meaningful. This response is not usable to submit a real transaction.

## FindPath (unary)

Same request/response shape as v1's `FindPath` (see [`v1.md`](./v1.md#findpath) for the full set of worked route-shape examples - `direct`, `indirect`, and `broker_swap` mean the same thing here), with two differences: `addresses` replaces `sender_address`/`receiver_address` as described above, and the response carries `response_code`/`required_chains`.

### FindPath - Request

- `chain_from`, `token_from_denom`, `amount_in`, `chain_to`, `token_to_denom` - same as v1.
- `addresses`: `repeated ChainAddress` - see above.
- `smart_route`, `slippage_bps` - same as v1.

### FindPath - Response

Same `direct`/`indirect`/`broker_swap` oneof as v1, plus:

- `response_code`: `RESPONSE_CODE_OK` for a real, executable response, or `RESPONSE_CODE_MOCK_ADDRESSES` for a discovery response (see above).
- `required_chains`: chain IDs the route touches (in mock mode, every chain that needs a real address).

## FindPathStreaming

`FindPathStreaming` is a bidirectional-streaming RPC with no v1 equivalent, for clients that want a route/quote that stays fresh without re-polling. It's only reachable over gRPC, gRPC-Web, or Connect's native streaming transport - the one-shot curl/HTTP GET-or-POST patterns from [`overview.md`](./overview.md) don't apply to it.

Unlike unary `FindPath`, `addresses` is always required here (minimum 2 entries) - there is no mock/discovery mode over a stream, since a client that's already set up a long-lived connection to watch a quote is presumably past the discovery phase.

### Runtime contract

The server enforces three timing rules:

1. **60-second first-message timeout.** The client must send its first `FindPathStreamingRequest` within 60 seconds of opening the stream, or the server closes it. This prevents idle goroutines from piling up on the server from clients that open a stream and never send anything.
2. **15-second idle auto-refresh.** After the server sends a response, if the client doesn't send a new request within 15 seconds, the server automatically recomputes the route from the *last request it received* and pushes an updated response - no new request needed from the client to keep the quote current.
3. **1-hour hard lifetime cap.** Regardless of activity, the server closes the stream after 1 hour. The client must reopen a new stream to continue.

### Example timeline

```
t=0s     client opens stream, sends FindPathStreamingRequest #1 (osmosis-1 -> noble-1, 5000000 uatom)
t=0.3s   server responds with response #1 (a broker_swap route)
t=~15s   client has sent nothing new; server auto-recomputes from request #1 and pushes response #2
t=22s    client sends FindPathStreamingRequest #2 (same route, amount_in bumped to 6000000)
t=22.3s  server responds with response #3
t=~37s   client idle again; server auto-recomputes and pushes response #4
  ...
t=3600s  server closes the stream (1-hour cap reached); client must open a new stream to continue
```

### Where this fits today

As of now, the Pathfinder frontend (`client_app`) only calls the unary v1 `FindPath` - there is no `FindPathStreaming` integration anywhere in this repo yet. The frontend's existing `usePathfinderQuery` hook (`client_app/src/hooks/usePathfinderQuery.ts`) approximates what `FindPathStreaming` is designed to eventually simplify: it debounces input changes, auto-refreshes on a timer, tracks staleness, and guards against race conditions between overlapping requests - all client-side logic that a long-lived stream with server-side auto-refresh could take over. Treat the description above as the API contract, not a description of current production behavior.

## TokenInfo denom field rename

v1's `TokenInfo` (used inside `GetChainInfo`'s `allowed_tokens` map) has a field called `ibc_denom`: the denom this token has on the *other end* of the route. v2beta renames this field to `counterpart_denom`. The rename is about intent, not behavior: the value isn't always a literal IBC hash - it can be a plain native denom when the token in question originates on the counterparty chain rather than the current one. `counterpart_denom` describes what the field actually holds more accurately than `ibc_denom` did.

## PathfinderQueryService methods

These mirror v1's query methods field-for-field, with two naming changes for consistency with the RPC method names: `ChainInfoRequest`/`ChainInfoResponse` become `GetChainInfoRequest`/`GetChainInfoResponse`, and `PathfinderSupportedChainsResponse` becomes `ListSupportedChainsResponse`. See [`v1.md`](./v1.md) for the full request/response field lists and worked JSON examples - only the differences are called out below.

### LookupDenom

Identical request/response shape to v1. See [`v1.md`](./v1.md#lookupdenom).

### GetTokenDenoms

Identical request/response shape to v1. See [`v1.md`](./v1.md#gettokendenoms).

### GetChainInfo

Request type is `GetChainInfoRequest` (same fields as v1's `ChainInfoRequest`: `chain_id`, `show_symbols`). Response type is `GetChainInfoResponse`. The nested `TokenInfo.counterpart_denom` field replaces v1's `ibc_denom` - see above. Otherwise identical to v1's example in [`v1.md`](./v1.md#getchaininfo).

### ListSupportedChains

Takes `google.protobuf.Empty`, same as v1. Response type is `ListSupportedChainsResponse` (v1 calls this `PathfinderSupportedChainsResponse`) with the same single `chain_ids` field. See [`v1.md`](./v1.md#listsupportedchains).

### GetChainTokens

Identical request/response shape to v1. See [`v1.md`](./v1.md#getchaintokens).

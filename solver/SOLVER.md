# The Spectra IBC Solver

The Solver is an information broker for token routing across different chains. The implementation routes tokens via IBC bridging with optional DEX swaps on broker chains. The solver provides a ConnectRPC service that can be queried to get the necessary information to bridge tokens.

## How it works

ConnectRPC provides multiple endpoints to query the solver: gRPC, gRPC-Web, and HTTP-Connect.

## Route Types

The solver attempts to find routes in priority order, returning the first successful match:

### 1. Direct Route

Basic IBC bridging from Chain A to Chain B with the same token.

**Requirements:**
- Direct IBC channel between source and destination chains
- Same token available on both chains (same origin)

**Flow:**
```text
Chain A -> [IBC Transfer] -> Chain B
```

**Response Type:** `direct`

---

### 2. Indirect Route (Multi-Hop)

Multiple IBC transfers through intermediate chains without token swaps. The same token travels through all chains.

**Requirements:**
- Multi-hop path exists between chains
- Same token (by origin) available on all intermediate chains
- Each hop has an IBC channel

**Flow:**
```
Chain A -> Chain B -> Chain C
```

**With PFM (Package Forwarding Middleware):**
If all intermediate chains support PFM, the entire route can be executed in a single transaction using IBC memo forwarding. The solver will:
- Detect PFM support on intermediate chains
- Generate the appropriate nested IBC memo
- Return `supports_pfm: true` with the memo

**Without PFM:**
Manual execution required - user must perform each IBC transfer sequentially.

**Response Type:** `indirect`

---

### 3. Broker Swap Route

Route through a broker chain (e.g., Osmosis) that performs a DEX swap to exchange tokens.

**Requirements:**
- Source chain can reach broker via IBC
- Broker has a DEX (only brokers marked with `Broker: true`)
- Broker can reach destination via IBC
- DEX has liquidity for the token pair

**Flow:**
```
Chain A -> [IBC Transfer] -> Broker -> [DEX Swap] -> [IBC Transfer] -> Chain C
```

**With PFM on Broker:**
If the broker supports PFM, the swap output can be automatically forwarded to the destination chain in a single transaction. The solver will:
- Query the broker DEX for swap quote
- Check if broker supports PFM
- Generate PFM memo for automatic forwarding of swap output
- Return `outbound_supports_pfm: true` with the memo

**Without PFM:**
Two transactions required:
1. Transfer to broker and swap
2. Transfer from broker to destination

**Response Type:** `broker_swap`

---

## Package Forwarding Middleware (PFM)

PFM allows chaining IBC transfers using memos, enabling multi-hop routes in a single transaction.

**How it works:**
- User initiates a single IBC transfer from Chain A
- Chain A includes a special memo with forwarding instructions
- Chain B receives the tokens and automatically forwards them to Chain C
- Process continues until tokens reach final destination

**Requirements:**
- For `A -> B -> C`, Chain B must support PFM (intermediate chains)
- First chain (A) only needs to send the memo
- Last chain (C) only receives, no PFM needed

**Memo Format:**
```json
{
  "forward": {
    "receiver": "cosmos1abc...",
    "port": "transfer",
    "channel": "channel-123"
  }
}
```

For multi-hop paths, memos are nested to specify the entire route.

---

## Route Priority

The solver tries routes in this order:

1. **Direct Route** - Fastest, no intermediate hops
2. **Indirect Route** - Multi-hop without swaps (prefers PFM when available)
3. **Broker Swap Route** - When token exchange is needed

This ensures the solver always returns the most efficient available route.


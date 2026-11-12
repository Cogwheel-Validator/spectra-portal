# pathfinder-sign

`pathfinder-sign` is a small program that runs **on your own computer** and lets **your browser wallet
(Keplr)** sign a transfer that an AI agent prepared through the Spectra Pathfinder MCP server.

- It holds **no keys**. Signing happens inside Keplr, after you review and approve.
- It needs **no Node or Python**: it is one self-contained executable.
- It only listens on `127.0.0.1` (your own machine) and exits when you are done.

Why it exists: see [MCP_SIGNING.md](MCP_SIGNING.md). In short, a browser wallet only works on pages
from origins it trusts, and `127.0.0.1` is one of them, while the sandbox a chat app uses is not.

## Install

Download the file for your system from the
[latest release](https://github.com/Cogwheel-Validator/spectra-portal/releases/latest):

| System | File |
| --- | --- |
| Linux (Intel/AMD) | `pathfinder-sign-<version>-linux-amd64` |
| Linux (ARM) | `pathfinder-sign-<version>-linux-arm64` |
| macOS (Intel) | `pathfinder-sign-<version>-darwin-amd64` |
| macOS (Apple silicon: M1/M2/M3...) | `pathfinder-sign-<version>-darwin-arm64` |
| Windows (64-bit) | `pathfinder-sign-<version>-windows-amd64.exe` |

Also download `pathfinder-sign-<version>-checksums.txt` and **verify the download**:

```bash
# Linux / macOS (on macOS use: shasum -a 256 -c ...)
sha256sum -c --ignore-missing pathfinder-sign-<version>-checksums.txt
```

```powershell
# Windows PowerShell: compare this value with the line for the .exe in the checksums file
Get-FileHash .\pathfinder-sign-<version>-windows-amd64.exe -Algorithm SHA256
```

Then put it on your `PATH` under the name `pathfinder-sign`:

```bash
# Linux / macOS
chmod +x pathfinder-sign-<version>-linux-amd64
mkdir -p ~/.local/bin && mv pathfinder-sign-<version>-linux-amd64 ~/.local/bin/pathfinder-sign
pathfinder-sign --version
```

On **macOS**, a downloaded unsigned binary is quarantined the first time. If macOS refuses to run it, remove the
quarantine flag once: `xattr -d com.apple.quarantine ~/.local/bin/pathfinder-sign`. (The binaries are not code-signed or
notarised yet; that is why verifying the checksum matters.)

On **Windows**, rename the file to `pathfinder-sign.exe` and put it in a folder on your `PATH`, or run it by its full
path. SmartScreen may warn about an unsigned program; that is expected for now.

You can also build it yourself: `make build-signer` (see [Building](#building)).

## How you normally use it

You do not run it by hand. You ask your AI assistant to move tokens; the assistant calls the Pathfinder MCP tool
`prepare_transfer`, which validates the route and returns a ready command. The assistant runs it (coding agents such
as Claude Code, Cursor or Codex can), or tells you to run it in a terminal (chat apps without a shell):

```text
pathfinder-sign --pathfinder-url https://pathfinder.example.com --chain-from cosmoshub-4 --token-from uatom --amount 1000000 --chain-to osmosis-1
```

Then:

1. Your browser opens a page at `http://127.0.0.1:<random port>/?t=<one-time token>`. If it does not open, copy the
   link printed in the terminal into **the browser that has the Keplr extension**.
2. Press **Connect Keplr** and approve the connection. The page reads your addresses from Keplr (the assistant does
   not need them).
3. **Review** the page: chains, amount, token, recipient and each message. The page builds this from the actual
   transaction data, not from the assistant's description. If the assistant chose the recipient, the page says so.
4. Press **Sign & send**, then approve in the Keplr popup. The page then shows the transaction hash and follows the
   transfer. Right after sending, the chain may not know the transaction yet (`not_found`); the page keeps waiting.
   For transfers needing several transactions it waits for each one to complete before asking for the next.
   It never signs a step twice: if something goes wrong after sending, the button becomes **Resume**, which only
   resumes waiting.
5. When the transfer completes the program prints a JSON result and exits:

```json
{"status":"completed","transactions":[{"chain_id":"cosmoshub-4","tx_hash":"9F2C..."}]}
```

The assistant then follows the transfer with `track_transfer`. If you ran the command yourself, paste that JSON into
the chat.

## Command reference

```text
pathfinder-sign --pathfinder-url URL --chain-from ID --token-from DENOM --amount N --chain-to ID [options]
pathfinder-sign --request '<json>' | @file | -        (same information as one JSON document)
```

| Flag | Meaning |
| --- | --- |
| `--pathfinder-url` | Base URL of the Pathfinder serving `/mcp` (https; plain http only for localhost). |
| `--chain-from`, `--chain-to` | Chain IDs, for example `cosmoshub-4`, `osmosis-1`. |
| `--token-from` | Denom, IBC denom, or `symbol@origin_chain`. |
| `--amount` | Amount in **base units** (1 ATOM = `1000000`). A positive integer. |
| `--token-to` | Destination token when swapping. Empty keeps the same token. |
| `--recipient` | Destination address. Leave out to send to your own address on the destination chain. |
| `--mode` | `smart` (default, fewest transactions) or `manual` (one transaction per leg). |
| `--slippage-bps` | Max swap slippage in basis points (default 100 = 1%). |
| `--port` | Local port. Default: a free one. |
| `--no-open` | Do not open the browser; only print the link. |
| `--timeout` | Give up after this long without a result. Default `10m`. |
| `--version` | Print the version. |

JSON request form (all fields as returned in `signing_request` by `prepare_transfer`):

```json
{"pathfinder_url":"https://pathfinder.example.com",
 "request":{"chain_from":"cosmoshub-4","token_from":"uatom","amount_in":"1000000","chain_to":"osmosis-1"}}
```

Output: **stdout** gets exactly one JSON line (so scripts and agents can parse it); progress and the link go to
**stderr**.

| `status` | Meaning | Exit code |
| --- | --- | --- |
| `completed` | All transactions were signed, sent and completed. `transactions` lists them. | 0 |
| `failed` | A transaction **was sent** but did not end well (for example the IBC packet failed or timed out). `transactions` has the hashes and `error` says why; funds are refunded on the sending chain. | 1 |
| `timeout` | Nothing was finished in time. If you already approved in Keplr, the transaction may still have been sent: check Keplr's history or an explorer for its hash. | 1 |
| `cancelled` | Interrupted (Ctrl+C). | 1 |
| `error` | Bad arguments or could not reach Pathfinder. | 2 |

## Security model

What it protects against and what it does not:

- **No keys.** Signing happens in Keplr. The program and the page never see a private key or mnemonic.
- **Loopback only.** It binds `127.0.0.1`. Other computers cannot reach it.
- **One-time link.** The page and every API call need a random token (24 random bytes) that only appears in the link
  printed in your terminal. Requests with a wrong `Host` or `Origin` are rejected, which blocks other websites and
  DNS-rebinding tricks from talking to the program while it runs.
- **Restricted proxy.** The page can only call these Pathfinder tools through the program: `find_route`,
  `build_transaction`, `build_sign_doc`, `assemble_signed_tx`, `broadcast_signed_tx`, `track_transfer`.
- **Strict page policy.** The page is served with a content security policy: no external scripts, styles or
  connections. It only talks back to the program.
- **Input is restricted.** The arguments come from an AI agent and are placed in a command you run, so chain IDs,
  denoms and addresses may only contain letters, digits and `/ _ . : @ -`. Anything else is refused.
- **Short-lived.** It exits after the result, or after `--timeout`.
- **The wallet is the final gate.** Nothing is signed without your click in Keplr. Reading the page before you approve
  is your protection against a wrong amount, chain or recipient. A compromised browser or wallet extension is out of
  scope.

## Requirements and limits

- The **Keplr** browser extension, in the browser that opens the link.
- Standard Cosmos accounts (secp256k1) and **Injective** accounts (`inj1...`, eth_secp256k1) signed with a software wallet. A Ledger on Injective is not supported.
- A Ledger through Keplr works on Cosmos chains (amino mode); it needs the Cosmos app open. It does not work on Injective.
- The chain must be known to Keplr (Keplr adds most Cosmos chains itself). Unknown chains are not suggested yet.
- The program must reach the Pathfinder URL over the network.
- Broadcasting goes through Keplr's own nodes, falling back to the Pathfinder `broadcast_signed_tx` tool if the
  operator enabled it.

## Troubleshooting

| Symptom | Fix |
| --- | --- |
| `pathfinder-sign: command not found` | It is not on your `PATH`. Use the full path, or move it to `~/.local/bin` (Linux/macOS). |
| macOS says the file cannot be opened | `xattr -d com.apple.quarantine <file>` once. |
| Page says *Keplr was not found* | Open the printed link in the browser where Keplr is installed. Some browsers block extensions on `http://127.0.0.1`: check the extension's site access. |
| Browser did not open | Copy the link from the terminal (stderr). Use `--no-open` on servers/SSH sessions and forward the port. |
| `error: could not connect to ...` | Wrong `--pathfinder-url`, or the server has MCP disabled (`enable_mcp = false`). |
| `unauthorized` in the browser | You opened an old or edited link. Use the exact link from the current run. |
| Account does not exist | The signing address has never received funds on that chain. Fund it first. |
| `timeout` | Nothing was approved within `--timeout`. Nothing was sent. Run it again. |
| IBC timeout while you were signing | Run `prepare_transfer` again; packets expire 10 minutes after the route was built. |

## Building

```bash
make build-signer                       # for this machine -> build/pathfinder-sign
make build-signer-all SIGNER_VERSION=1.2.3   # Linux, macOS, Windows + checksums -> build/
```

Individual targets: `build-signer-linux-amd64`, `build-signer-linux-arm64`, `build-signer-darwin-amd64`,
`build-signer-darwin-arm64`, `build-signer-windows-amd64`, `signer-checksums`, `clean-signer`.
They are pure Go (`CGO_ENABLED=0`), so any host can build all of them. Release builds
(`.github/workflows/release.yml`) run `make build-signer-all` on every `v*` tag and attach the binaries and
the checksums file to the GitHub release.

## For developers

- `signer/` is the library (HTTP server, request validation, MCP client). `signer/cmd` is the command. `signer/web` embeds
  the page (`signer.html`).
- The same page is also used by the experimental inline MCP view. It picks its transport at start: inside an
  MCP Apps host it talks through the host (`postMessage`), and under `pathfinder-sign` the program injects
  `window.__SPECTRA_LOCAL__` and it talks over `fetch` to `/api/*`.
- Local API (all need the `X-Spectra-Token` header): `GET /api/request` (the transfer plus a route preview),
  `POST /api/call` (allowlisted tool proxy), `POST /api/result` (completes the run).
- Tests: `go test ./signer/...` (token, host/origin checks, allowlist, result, timeout) and
  `go test ./pathfinder/mcp/` (including a real client against the real MCP handler).

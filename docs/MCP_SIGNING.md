# How signing works with the Pathfinder MCP

This document explains **why** the MCP signs the way it does, what was tried, and what the limits are. For
setup and tools see [MCP.md](MCP.md); for the signing program see [PATHFINDER_SIGN.md](PATHFINDER_SIGN.md).

## The principle

The Pathfinder is an online service. It must never hold a key or sign. It **plans, builds and tracks**; the
user's own wallet signs. That split is what makes the MCP useful and safe:

| Layer | Who | What |
| --- | --- | --- |
| Plan | MCP (server) | find tokens and routes, quote swaps, explain options |
| Build | MCP (server) | unsigned messages, sign docs (account number, sequence, simulated gas, fee) |
| **Sign** | **User's wallet** | approve and sign |
| Send | Wallet or MCP | broadcast the signed transaction (MCP only if the operator enabled it) |
| Track | MCP (server) | follow IBC packets across chains until completed / failed / timed out |

Everything except signing works for any client with no wallet at all, including bots and other agents.

## The problem: a wallet needs a page it trusts

A browser wallet such as Keplr only injects itself (`window.keplr`) into web pages on origins it considers
valid, and it is the **page** that asks the wallet to sign. An AI chat is not such a page, so something has to be a
page the wallet will talk to. The options:

1. **An inline UI inside the chat** (MCP Apps, `ui://` resources). The MCP server sends an HTML page and the chat app
   renders it in a sandboxed iframe.
2. **A page hosted by Pathfinder** that the user opens from a link.
3. **A page served from the user's own machine** (`127.0.0.1`).
4. **No browser wallet**: a CLI (`gaiad`, `osmosisd`), a hardware signer, or WalletConnect.
5. **The agent holds a key** (or an authz grant). Rejected for the default design: the guide forbids handling keys.

### What we found with the inline UI (option 1)

We built it (the signer page, registered as `ui://spectra/signer` with `text/html;profile=mcp-app`) and tried it in a
browser-based chat host:

- The agent called the tool and the view rendered, but `window.keplr` never appeared.
- The browser console showed `Untrusted origin: https://<hash>.claudemcpcontent.com`. The host runs MCP Apps on its own
  random sandbox origin, and the extension side refused it. That origin is outside our control (it is different for
  every piece of content), so there is nothing to allow-list from the server.
- It is not known which extension printed that message (the line came from a file named `content.js`, not Keplr's
  `contentscript.js`), so this is a finding about that host and that browser setup, not a universal rule. Other hosts
  (desktop apps, IDEs) may behave differently. We did not test them.

So the inline view is kept as an **experimental, opt-in** feature (`enable_mcp_inline_ui`, default off) and the
product does not depend on it. When it fails, it prints diagnostics (frame, origin, whether Keplr exists, CSP
violations) so the cause is visible.

> The MCP Apps protocol messages used by the inline view (`ui/initialize`, `ui/message`, `ui/update-model-context`,
> `tools/call` through the host) were written from the SEP-1865 design and **not checked against the final
> specification text**; only the handshake and the MIME type were confirmed. Treat the inline view as unverified.

### Why not a Pathfinder-hosted signing page (option 2)

It works with every client, but it makes the MCP depend on a UI run by whoever hosts the Pathfinder, adds a public
page and short-lived sessions to the server, and at that point sending the user to the dApp is simpler. We decided not
to build it. If a chat-only user needs one-click signing, the right answer today is the dApp itself.

### What we chose (options 3 and 4)

**Primary: `pathfinder-sign`, a small local program (option 3).**

```text
 Agent / chat ──MCP──►  Pathfinder (remote)      finds the route, builds txs, tracks
      │                      ▲
      │ runs one command     │ HTTPS (MCP)
      ▼                      │
 pathfinder-sign (user's PC) ───┘
   serves the page on 127.0.0.1 ──► user's browser tab ──► Keplr popup (user approves)
```

The remote server stays key-free and stateless. The only thing on the user's machine is a signing go-between that
holds no keys. The page and the build/sign-doc logic are the same ones the server already has, so there is one
implementation of the transaction encoding (`pathfinder/mcp/cosmostx`, tested against CosmJS).

**Fallback: manual signing (option 4).** The sign-doc tools (`build_sign_doc`, `assemble_signed_tx`) stay, so a
user with a CLI wallet or a Ledger can sign the sign doc themselves and give back the 64-byte signature.

### Client matrix

| Client | Plan / build / track | Sign |
| --- | --- | --- |
| Claude Code, Cursor, Codex (a shell is available) | yes | Agent runs `pathfinder-sign`; user approves in Keplr |
| Claude Desktop, ChatGPT and other chat apps without a shell | yes | Agent hands the user the command; user runs it and pastes the JSON back |
| Chat apps in a browser where the user cannot run programs | yes | Not possible in-chat; use the dApp or a CLI wallet |
| Any client + CLI wallet / Ledger | yes | Manual flow with `build_sign_doc` |
| Experimental inline view (opt-in) | yes | Only if the host's origin is trusted by the wallet extension |

### Interactions per transfer

Target: the user speaks once and clicks approve once.

1. The prompt.
2. Press **Connect Keplr** and **Sign & send** in the page, then approve in Keplr.

Where interactions were removed compared with the original manual flow:

- **One call plans everything.** `prepare_transfer` validates the route and returns the command, so the agent does not
  chain `find_route` → ask for addresses → `build_transaction` → `build_sign_doc` → `assemble_signed_tx` → broadcast.
- **No addresses to type.** The page reads them from Keplr.
- **The signature is the confirmation.** Nothing needs a second "yes" in chat after the user has reviewed the page and
  approved in Keplr. The agent still shows the plan first (guide rule 2), because that is the last chance to correct
  a wrong chain or amount.
- **The wallet broadcasts.** Keplr sends the transaction through its own nodes, so it works even when the operator did
  not enable `broadcast_signed_tx`.
- **Per-tool prompts are not controllable from here.** Some clients ask for approval of each tool call. Marking tools
  read-only helps; users can usually set "always allow" for this server.

## Prior art

Other blockchain MCP servers have hit the same wall. They all solve it locally; none solved it for a remote
server inside a browser chat. (Summarised from public READMEs and directory listings; not audited.)

| Project | Approach |
| --- | --- |
| [browser-web3-signer](https://github.com/nikicat/browser-web3-signer) (EVM, Tron, Bitcoin) | Local CLI serving an approval page on `127.0.0.1`; the extension signs. It is not an MCP server; agents run it as a command. This is the model `pathfinder-sign` follows. |
| [mcp-wallet-signer](https://github.com/nikicat/mcp-wallet-signer) | Stdio MCP wrapper over the same idea. Reported not to work with remote HTTP MCP. |
| [WalletChan MCP](https://github.com/walletchan/walletchan-mcp) | Local stdio MCP; pairs a wallet via WalletConnect / MetaMask Connect; popup per transaction. Optional delegation (ERC-7710) removes the popup. |
| [Keplr Wallet MCP](https://mcp.keplr.app/docs/reference) | Local stdio MCP with its own encrypted mnemonic in the OS keychain. A different trust model (a key exists on the machine). |
| web3-mcp (hosted) | A hosted signing web app opened by link: the option we declined. |

## Future options

- **WalletConnect v2** (phone approval) needs a client session held somewhere; locally it could be part of
  `pathfinder-sign`.
- **Authz / fee grants** would let an agent-held key act within a user-set limit. This is attractive for bots, but it
  puts a key with the agent and contradicts the default safety rules, so it would be a separate, explicit opt-in design.
- **A prefilled dApp link** (the client app has no prefill support yet) as the fallback for chat-only users.
- Code-signed and notarised `pathfinder-sign` builds for macOS and Windows.

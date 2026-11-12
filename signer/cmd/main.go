// Command pathfinder-sign signs a Spectra transfer with the user's browser wallet.
//
// An AI agent (or the user) gets a signing request from the Pathfinder MCP tool
// prepare_transfer and passes it here. The program serves a page on 127.0.0.1,
// opens it in the browser, the user approves in Keplr, and the transaction
// hashes are printed to stdout as JSON. It never sees a private key.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/signer"
)

var version = "dev"

func main() { os.Exit(run()) }

func run() int {
	var (
		reqArg    = flag.String("request", "", `signing request JSON, "@file" to read a file, or "-" for stdin (alternative to the flags below)`)
		chainFrom = flag.String("chain-from", "", "source chain ID")
		tokenFrom = flag.String("token-from", "", "denom, IBC denom, or symbol@origin_chain")
		amount    = flag.String("amount", "", "amount in base units, e.g. 1000000 for 1 ATOM")
		chainTo   = flag.String("chain-to", "", "destination chain ID")
		tokenTo   = flag.String("token-to", "", "destination token (empty keeps the same token)")
		recipient = flag.String("recipient", "", "destination address (default: your own address from the wallet)")
		mode      = flag.String("mode", "", "smart (default) or manual")
		slippage  = flag.Uint("slippage-bps", 0, "max swap slippage in basis points")
		urlArg    = flag.String("pathfinder-url", "", "override the Pathfinder base URL from the request")
		port      = flag.Int("port", 0, "local port (default: a free one)")
		noOpen    = flag.Bool("no-open", false, "do not open the browser; just print the link")
		timeout   = flag.Duration("timeout", 10*time.Minute, "give up if nothing is signed in this time")
		showVers  = flag.Bool("version", false, "print the version and exit")
	)
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: pathfinder-sign --pathfinder-url URL --chain-from ID --token-from DENOM --amount N --chain-to ID [--token-to DENOM] [--recipient ADDR]\n       pathfinder-sign --request '<json>'  (alternative form)\n\nSigns a transfer prepared by the Pathfinder MCP tool prepare_transfer using your browser wallet (Keplr).\nThe result is printed to stdout as JSON; progress goes to stderr.")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVers {
		fmt.Println("pathfinder-sign", version)
		return 0
	}
	var req signer.Request
	switch {
	case *reqArg != "":
		raw, err := readRequest(*reqArg)
		if err != nil {
			return fail(err)
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return fail(fmt.Errorf("request is not valid JSON: %w", err))
		}
	case *chainFrom != "":
		if *slippage > 10000 {
			return fail(fmt.Errorf("--slippage-bps must be at most 10000, got %d", *slippage))
		}
		req.Transfer = signer.Transfer{
			ChainFrom: *chainFrom, TokenFrom: *tokenFrom, AmountIn: *amount, ChainTo: *chainTo,
			TokenTo: *tokenTo, Recipient: *recipient, Mode: *mode, SlippageBps: uint32(*slippage), //nolint:gosec // bounded to 10000 above
		}
	default:
		flag.Usage()
		return 2
	}
	if *urlArg != "" {
		req.PathfinderURL = *urlArg
	}
	if err := req.Validate(); err != nil {
		return fail(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	remote, err := signer.Dial(ctx, req.PathfinderURL, version)
	if err != nil {
		return fail(err)
	}
	defer remote.Close() //nolint:errcheck

	srv, err := signer.New(&req, remote)
	if err != nil {
		return fail(err)
	}
	link, err := srv.Listen(*port)
	if err != nil {
		return fail(err)
	}
	t := req.Transfer
	fmt.Fprintf(os.Stderr, "Transfer: %s %s from %s to %s\n", t.AmountIn, t.TokenFrom, t.ChainFrom, t.ChainTo)
	if t.Recipient != "" {
		fmt.Fprintf(os.Stderr, "Recipient (set by the agent, verify it in the page): %s\n", t.Recipient)
	}
	fmt.Fprintf(os.Stderr, "Open this link in the browser that has Keplr installed:\n  %s\n", link)
	if !*noOpen {
		if err := openBrowser(ctx, link); err != nil {
			fmt.Fprintln(os.Stderr, "could not open the browser automatically:", err)
		}
	}

	res := srv.Serve(ctx, *timeout)
	out, _ := json.Marshal(res)
	fmt.Println(string(out))
	if res.Status != "completed" {
		return 1
	}
	return 0
}

func fail(err error) int {
	out, _ := json.Marshal(signer.Result{Status: "error", Error: err.Error()})
	fmt.Println(string(out))
	fmt.Fprintln(os.Stderr, "error:", err)
	return 2
}

func readRequest(arg string) ([]byte, error) {
	switch {
	case arg == "-":
		return io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	case strings.HasPrefix(arg, "@"):
		return os.ReadFile(arg[1:])
	default:
		return []byte(arg), nil
	}
}

// openBrowser opens our own loopback link; the argument is never user input,
// so the variable subprocess arguments below are safe.
func openBrowser(ctx context.Context, link string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.CommandContext(ctx, "open", link).Start() //nolint:gosec // link is our loopback URL
	case "windows":
		return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", link).Start() //nolint:gosec // link is our loopback URL
	default:
		return exec.CommandContext(ctx, "xdg-open", link).Start() //nolint:gosec // link is our loopback URL
	}
}

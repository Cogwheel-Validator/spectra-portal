# Pathfinder

Spectra's Pathfinder RPC provides a set of queries to help you find the best route to bridge tokens between two chains. The Pathfinder is powered by ConnectRPC, written in Go. This allows the Pathfinder to be accessible via 3 protocols: gRPC, gRPC-Web, and HTTP-Connect.

This document covers the parts that are common to every API version: the protocols and tools you can use to query the RPC. For the actual methods and worked examples, see:

- [`v1.md`](./v1.md) - the `pathfinder.v1.PathfinderService` API (legacy, kept for existing consumers)
- [`v2beta.md`](./v2beta.md) - the `pathfinder.v2beta.FindPathService` / `pathfinder.v2beta.PathfinderQueryService` API (recommended for new integrations)

## Which API version should I use?

**New integrations should use v2beta.** v1 identifies every address involved in a route with a single `sender_address`/`receiver_address` pair, and derives any intermediate broker/PFM chain address automatically via SLIP-44-guarded bech32 re-encoding. That derivation is a source of subtle bugs across chains that use different SLIP-44 coin types. v2beta replaces it with an explicit, caller-supplied `ChainAddress` list - one address per chain the route touches - and adds two capabilities v1 doesn't have: a mock/discovery response mode (see [`v2beta.md`](./v2beta.md)) and a bidirectional-streaming `FindPathStreaming` RPC for quotes that stay fresh without polling.

v1 is kept unchanged for existing consumers and is not going away on short notice, but it should be considered legacy.

## How to query the RPC?

ConnectRPC allows multiple methods to query the RPC. Not every method will be covered here - if you want to get more information on all of the methods and clients available you can find them in ConnectRPC's [documentation](https://connectrpc.com/docs/introduction/).

### HTTP-Connect

HTTP-Connect is the easiest way to query the Pathfinder RPC. It is a simple HTTP request to the RPC endpoint. It allows you to make queries to the RPC using a simple HTTP request. No need to install any libraries or to generate any protobuf files.

But if you want to get the best result without the need to parse the JSON response, you can generate the proto code and still use HTTP-Connect.

#### POST Method

If you do not want to use any kind of ConnectRPC client you can always just make a simple HTTP POST request to the RPC endpoint.

For a simple curl request you can do something like this to get the chain info for Juno (using the v2beta `PathfinderQueryService`):

```bash
curl -X POST https://pathfinder.thespectra.io/pathfinder.v2beta.PathfinderQueryService/GetChainInfo \
-H "Content-Type: application/json" \
-H "Accept: application/json" \
-d '{
  "chain_id": "juno-1"
}'
```

For a frontend application you can import `@connectrpc/connect-web` or you can always just use the `fetch` API.

```typescript
const response = await fetch(
  "https://pathfinder.thespectra.io/pathfinder.v2beta.PathfinderQueryService/GetChainInfo",
  {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ chain_id: "juno-1" }),
  },
);
const data = await response.json();
console.log(data);
```

For something like Python and Go it might look like this:

```python
import requests

def get_chain_info(chain_id: str):
    response = requests.post(
        "https://pathfinder.thespectra.io/pathfinder.v2beta.PathfinderQueryService/GetChainInfo",
        json={"chain_id": chain_id},
        headers={"Content-Type": "application/json", "Accept": "application/json"}
    )
    return response.json()

if __name__ == "__main__":
    print(get_chain_info("juno-1"))
```

```go
package main

import (
  "bytes"
  "fmt"
  "io"
  "net/http"
)

func main() {
  response, err := http.Post("https://pathfinder.thespectra.io/pathfinder.v2beta.PathfinderQueryService/GetChainInfo", "application/json", bytes.NewBuffer([]byte(`{"chain_id": "juno-1"}`)))
  if err != nil {
    fmt.Println(err)
    return
  }
  defer response.Body.Close()
  body, err := io.ReadAll(response.Body)
  if err != nil {
    fmt.Println(err)
    return
  }
  fmt.Println(string(body))
}
```

For some programming languages there might exist a package that can make some of these queries a bit easier to make, so be sure to check the official ConnectRPC documentation for more information.

#### GET Method

All of the current RPC methods use idempotency level `NO_SIDE_EFFECTS`. This means the query is idempotent and will almost always return the same result, which means you can cache the request and return the cached result in most cases.

The one exception is `FindPath`: it is technically idempotent, but it can return a different result even for the same request. This mostly depends on the underlying Osmosis SQS - it might return a different result if it finds a better trade pair, but that doesn't mean the route is meaningfully different or that the trade pair is that much better. Take a cached `FindPath` result with a grain of salt unless it's fresh, or keep caching to well under a minute if the requested route is the exact same as the cached route.

If you are not sure, it's better not to cache `FindPath` and just make a fresh request every time. For every other method you can cache the request and return the cached result in most cases.

To make a GET request you need to encode the data into the URL:

```bash
curl --get --data-urlencode 'encoding=json' \
    --data-urlencode 'message={"chain_id": "juno-1"}' \
    https://pathfinder.thespectra.io/pathfinder.v2beta.PathfinderQueryService/GetChainInfo
```

There is another alternative: include the data within the URL directly. You can access this request via a browser at `https://pathfinder.thespectra.io/pathfinder.v2beta.PathfinderQueryService/GetChainInfo?encoding=json&message={"chain_id"%3a"juno-1"}` or via curl:

```bash
curl --get -H "Accept: application/json" --data-urlencode 'encoding=json' \
--data-urlencode 'message={"chain_id": "juno-1"}' \
https://pathfinder.thespectra.io/pathfinder.v2beta.PathfinderQueryService/GetChainInfo
```

For TypeScript you can use the `fetch` API like this:

```typescript
const url = new URL(
  "https://pathfinder.thespectra.io/pathfinder.v2beta.PathfinderQueryService/GetChainInfo",
);
url.searchParams.set("encoding", "json");
url.searchParams.set("message", JSON.stringify({ chain_id: "juno-1" }));
const response = await fetch(url);
const data = await response.json();
console.log(data);
```

### gRPC, gRPC-Web and HTTP-Connect with protobuf

The gRPC and gRPC-Web protocols rely on the protobuf files to generate code you can leverage in your own projects. This documentation will not go in depth on how to make clients for each programming language - check the [gRPC docs](https://grpc.io/docs/) for more information. However, here you will find out how to generate the code in the most efficient way, and get some help on how to test out gRPC via `grpcurl` and `grpcui` to gain a better insight on how to use the gRPC protocol.

Buf provides the best way to generate the code for the protobuf files. You can generate code for the protobuf files by creating a `buf.gen.yaml` in your project and running `buf generate`.

Here is an example `buf.gen.yaml` that matches what this repo actually uses (see `proto/buf.gen.yaml`) to generate a TypeScript client with full Connect support:

```yaml
version: v2
plugins:
  - local: protoc-gen-es
    out: ./src/lib/generated
    opt: target=ts
    include_imports: true
  - local: protoc-gen-connect-es
    out: ./src/lib/generated
    opt: target=ts
inputs:
  - git_repo: https://github.com/Cogwheel-Validator/spectra-portal
    branch: main
    subdir: proto
```

Both `protoc-gen-es` (message types) and `protoc-gen-connect-es` (the service client object, e.g. `PathfinderQueryService`) are required - `createClient()` needs the connect-es output, not just the plain message types. This config requires the buf CLI and the two npm-installed plugins above (`@bufbuild/protoc-gen-es`, `@connectrpc/protoc-gen-connect-es`); you don't need `protoc` itself. For other languages, swap in the matching protoc plugin - check the [buf docs](https://buf.build/docs/cli/#configuration-files) for more information, and check this repo's `proto/buf.gen.yaml` for the canonical, always-up-to-date generation config.

Note: if you regenerate from scratch, the header comment in freshly-generated files may show a newer `protoc-gen-connect-es` version than what's checked into this repo at any given time - that's expected, not a bug, and doesn't change how the generated client is used.

#### HTTP-Connect with protobuf

In case you want to have type-safe code in your app, you can generate the code and then import the client and use it.

```typescript
import { PathfinderQueryService } from "./lib/generated/pathfinder/v2beta/pathfinder_v2beta_query_pb";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

const transport = createConnectTransport({
  baseUrl: "https://pathfinder.thespectra.io",
});

const client = createClient(PathfinderQueryService, transport);

const chainInfo = await client.getChainInfo({ chainId: "juno-1" });
console.log(chainInfo.chainInfo?.chainName);
```

#### CLI and GUI tools

To test out the gRPC protocol you can use the `grpcurl` command line tool. You can find installation instructions at [grpcurl.com](https://grpcurl.com/#Installation%20Method).

To make a basic CLI request you can do something like this:

```bash
grpcurl -d '{"chain_id": "juno-1"}' pathfinder-grpc.thespectra.io:443 pathfinder.v2beta.PathfinderQueryService/GetChainInfo
```

To get a list of all the methods you can do something like this:

```bash
grpcurl pathfinder-grpc.thespectra.io:443 list
```

To make a basic GUI request you can use the `grpcui` tool. You can find installation instructions on the [grpcui GitHub page](https://github.com/fullstorydev/grpcui).

To get a UI to test out the RPC you can initiate it like this:

```bash
grpcui pathfinder-grpc.thespectra.io:443
```

A pop up in your browser should appear and you should be able to see the RPC methods and their requests. You can find more information about how this works in this [blog post](https://www.fullstory.com/blog/grpcui-dont-grpc-without-it/).

**Buf Studio** ([buf.build/studio](https://buf.build/studio)) is another option: it's a browser-based client that calls live gRPC/Connect/gRPC-Web APIs, without needing to install anything locally. Because raw gRPC can't run directly in a browser, it works through the Buf Studio Agent (bundled in the `buf` CLI) as a thin bridge to your server, and it can use either a schema published to the Buf Schema Registry or manually-provided proto descriptors. This repo doesn't currently publish to the BSR, so using Buf Studio against Pathfinder today means providing descriptors manually - `grpcurl`/`grpcui` or plain HTTP-Connect (curl, above) remain the zero-setup options for ad-hoc testing.

### Denom syntax

For any field documented as accepting a token denom (`token_from_denom`, `token_to_denom`, `denom` on `LookupDenom`, etc.), the Pathfinder accepts three forms:

- The token's original on-chain denom. This can be a native denom like `ujuno`, `uosmo`, `ustars`, etc., or a full IBC denom like `ibc/ABC123...`.
- The `symbol@origin_chain` convenience syntax, e.g. `uatone@atomone-1` or `uosmo@osmosis-1`. This names the chain a base denom is native to, which disambiguates it when the same base denom could otherwise resolve ambiguously.

The Pathfinder will automatically resolve any of these forms to the canonical chain-specific denom.

## See also

- [`v1.md`](./v1.md) and [`v2beta.md`](./v2beta.md) - the method-by-method reference for each API version.
- [`../IBC_MEMO.md`](../IBC_MEMO.md) - once you have a `broker_swap` route back from `FindPath`, this explains how the `execution.memo`/`execution.smart_contract_data` fields assemble into an IBC-hooks memo you can actually submit on-chain.
- `../openapi/v1.openapi.yaml` / `../openapi/v2beta.openapi.yaml` - machine-readable OpenAPI 3.1 specs generated from the same proto definitions (via `make generate-openapi`), useful if you want to feed the API into an OpenAPI-aware tool.

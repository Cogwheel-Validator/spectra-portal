# Pathfinder Queries

The Spectra's Pathfinder RPC provides a set of queries to help you find the best route to bridge tokens between two chains. The Pathfinder is powered by the ConnectRPC written in Go.
This allows the Pathfinder to be accessable via 3 protocols: gRPC, gRPC-Web, and HTTP-Connect.

## How to query the RPC?

ConnectRPC allows multiple methods to query the RPC, not every method will be covered here, if you want to get more information on all of the methods and clients available you can find them in ConnectRPC's [documentation](https://connectrpc.com/docs/introduction/).

### HTTP-Connect

HTTP-Connect is the easiest way to query the Pathfinder RPC. It is a simple HTTP request to the RPC endpoint.
It allows you to make queries to the RPC using a simple HTTP request. No need to install any libraries or to generate any protobuf files.

But if you want to get the best result without the need to parse the JSON response, you can generate to proto code and still use HTTP-Connect.

#### POST Method

If you do not want to use any kind or ConnectRPC client you can always just make a simple HTTP POST request to the RPC endpoint.

For a simple curl request you can do something like this to get the chain info for Juno:

```bash
curl -X POST https://pathfinder.thespectra.io/pathfinder.v1.PathfinderService/GetChainInfo \
-H "Content-Type: application/json" \
-H "Accept: application/json" \
-d '{
  "chain_id": "juno-1"
}'
```

For a frontend application you can import the  `@connectrpc/connect-web` or you can always just use the `fetch` API.

```typescript
const response = await fetch("https://pathfinder.thespectra.io/pathfinder.v1.PathfinderService/GetChainInfo", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "Accept": "application/json"
  },
  body: JSON.stringify({ chain_id: "juno-1" })
});
const data = await response.json();
console.log(data);
```

For something like Python and Go it might look like this:

```python
import requests

def get_chain_info(chain_id: str):
  response = requests.post(
      "https://pathfinder.thespectra.io/pathfinder.v1.PathfinderService/GetChainInfo",
      json={"chain_id": "juno-1"},
      headers={"Content-Type": "application/json", "Accept": "application/json"}
    )
    return response.json()

if __name__ == "__main__":
  print(get_chain_info("juno-1"))

```

```go
package main

import (
  "net/http"
  "bytes"
  "fmt"
  "io"
)

func main() {
  response, err := http.Post("https://pathfinder.thespectra.io/pathfinder.v1.PathfinderService/GetChainInfo", "application/json", bytes.NewBuffer([]byte(`{"chain_id": "juno-1"}`)))
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

For some programming languages there might exist a package that can make some of this queries a bit easier to make so be sure to check the official ConnectRPC documentation for more information.

#### GET Method

All of the current RPC methods use idempotency level of NO_SIDE_EFFECTS. This means that the query is idempotent and will ALMOST(read more below) always return the same result. Which means you cache the request and return the cached result in most cases.

All of the method except for FindPath always return the same result. The FindPath method is technically
idenpotent but it can return a different result even for the same request. This mostly depends on the
underlying Osmosis SQS. The SQS might return a different result if it finds a trade better pair but it doesn't
mean that the route is different or that the trade pair might be that much better. So take it with a grain of
salt unless it is a fresh request or keep caching to under a minute if the REQUESTED ROUTE IS EXACT SAME AS
THE CACHED ROUTE.

If you are not sure better to not cache the request and just make a fresh request every time. For every other method you can cache the request and return the cached result in most cases.

To make a GET request you need to encode the data into the URL:

```bash
curl --get --data-urlencode 'encoding=json' \
    --data-urlencode 'message={"chain_id": "juno-1"}' \
    https://pathfinder.thespectra.io/pathfinder.v1.PathfinderService/GetChainInfo
```

There is another alternative to include the data within the url. You can access this request via browser `https://pathfinder.thespectra.io/pathfinder.v1.PathfinderService/GetChainInfo?encoding=json&message={"chain_id"%3a"juno-1"}` or you can do it via curl:

```bash
curl  --get -H "Accept: application/json" --data-urlencode 'encoding=json' \
--data-urlencode 'message={"chain_id": "juno-1"}' \
https://pathfinder.thespectra.io/pathfinder.v1.PathfinderService/GetChainInfo
```

For typescript you can use the `fetch` API like this:

```typescript
const url = new URL("https://pathfinder.thespectra.io/pathfinder.v1.PathfinderService/GetChainInfo");
url.searchParams.set("encoding", "json");
url.searchParams.set("message", JSON.stringify({"chain_id": "juno-1"}));
const response = await fetch(url);
const data = await response.json();
console.log(data);
```

### gRPC, gRPC-Web and HTTP-Connect with protobuf

The gRPC and gRPC-Web protocols are protocols that rely on the protobuf files to generate the code which you can leverage in your own projects. This documentation will not go in depth on how to make clients for each programming language you can check the [docs](https://grpc.io/docs/) for more information. However here you will find out how to generate the code in most efficient way, and to give you some help on how to test out gRPC via grpcurl and grpcui to gain a better insight on how to use the gRPC protocol.

Buf provides the best way to generate the code for the protobuf files. You can generate the code for the protobuf by creating a `buf.gen.yaml` in your project and running `buf generate`.

Here is an example of a `buf.gen.yaml` file:

```yaml
version: v2
plugins:
  - local: protoc-gen-es
    opt: target=ts
    out: ../path/to/your/project/src
inputs:
  - git_repo: https://github.com/Cogwheel-Validator/spectra-ibc-hub
    branch: main
    subdir: proto
```

The file above will acquire the proto files directly from the github repository. This config does require that
you do have installed the protoc compiler and the buf CLI. You can also use any protoc directly from the buf
by using @bufbuild/protoc-gen-es. If you do need to generate the code for some other language you can just use
another protoc compiler. You can also use the protoc compiler to directly generate the code but buf provides a
better way to generate the code for the protobuf files. You can check the [docs](https://buf.build/docs/cli/#configuration-files)
for more information and also check the Github repo where the proto files are stored to gain some insight on
how to replicate it for your own project.

#### HTTP-Connect with protobuf

So in case you want to have a type safe code in your app you can generate the code, and then import the client and use it in your app.

```typescript
import { PathfinderService } from "../path/to/your/project/src/pathfinder_route_pb";
import { createConnectTransport } from "@connectrpc/connect-web";

const transport = createConnectTransport({
  baseUrl: "https://pathfinder.thespectra.io",
});

const client = createClient(PathfinderService, transport);

const chainInfo = await client.getChainInfo({ chain_id: "juno-1" });
console.log(chainInfo.chainName);
```

#### CLI and GUI tools

To test out the gRPC protocol you can use the `grpcurl` command line tool. You can find instructions to install at this [website](https://grpcurl.com/#Installation%20Method).

To make a basic CLI request you can do something like this:

```bash
grpcurl -d '{"chain_id": "juno-1"}' pathfinder.thespectra.io pathfinder.v1.PathfinderService/GetChainInfo
```

To get list of all the methods you can do something like this:

```bash
grpcurl pathfinder.thespectra.io list
```

To make a basic GUI request you can use the `grpcui` tool. You can find instructions to install at this [website](https://github.com/fullstorydev/grpcui).

To get a UI to test out the RPC you can initiate like this:

```bash
grpcui pathfinder.thespectra.io
```

A pop up in your browser should pop up and you should be able to see the RPC methods and their requests.
Some more data about how does this work you can check in this [blog post](https://www.fullstory.com/blog/grpcui-dont-grpc-without-it/).

## Methods

List of all methods:

- rpc.v1.PathfinderService
  - FindPath
  - LookupDenom
  - GetTokenDenoms
  - GetChainInfo
  - ListSupportedChains
  - GetChainTokens

### FindPath

The FindPath query is used to find the best route to bridge tokens between two chains. It will return a best route to the designated chain.

#### FindPath - Request

The request should contain the following fields:

- chain_from: The chain ID of the source chain.
- token_from_denom: The denom of the token on the source chain.
- amount_in: The amount of the token to bridge.
- chain_to: The chain ID of the destination chain.
- token_to_denom: The denom of the token on the destination chain.
- sender_address: The address of the sender.
- receiver_address: The address of the receiver.
- smart_route: Whether to return a smart route or a normal route(optional, default is false).
- slippage_bps: The slippage in basis points(optional and only applicable if the swap is required, default is 100).

To give additional flexibility, the token denoms can be entered in 2 different ways:

- Their original on chain denom. This can be a native denom like `ujuno`, `uosmo`, `ustars`, etc. or if the denom is an IBC denom like `ibc/ABC123...`.
- Using their original chain denom but by adding additional information of the token origin chain. This looks like this `uatone@atomone-1`, or `uosmo@osmosis-1`.

#### FindPath - Response

There are few wariations of the repsones you can get back from the RPC.

#### Direct Route

This is a simple IBC transfer route between the two chains. It will return a single route to the destination chain. Example:

```json
{
  "success": true,
  "error_message": "",
  "direct": {
    "transfer": {
      "from_chain": "osmosis-1",
      "to_chain": "atomone-1",
      "channel": "channel-94814",
      "port": "transfer",
      "token": {
        "chain_denom": "ibc/BC26A7A805ECD6822719472BCB7842A48EF09DF206182F8F259B2593EB5D23FB",
        "base_denom": "uatone",
        "origin_chain": "atomone-1",
        "is_native": false
      },
      "amount": "100000"
    }
  }
}
```

#### Osmosis Swap

This can occure if both the source and destination chains are Osmosis. It will return a single route to the destination chain. Example:

```json
{
  "success": true,
  "error_message": "",
  "broker_swap": {
    "path": [
      "osmosis-1"
    ],
    "inbound_leg": null,
    "swap": {
      "broker": "osmosis-sqs",
      "token_in": {
        "chain_denom": "uosmo",
        "base_denom": "uosmo",
        "origin_chain": "osmosis-1",
        "is_native": true
      },
      "token_out": {
        "chain_denom": "ibc/BC26A7A805ECD6822719472BCB7842A48EF09DF206182F8F259B2593EB5D23FB",
        "base_denom": "uatone",
        "origin_chain": "atomone-1",
        "is_native": false
      },
      "amount_in": "100000",
      "amount_out": "10258",
      "price_impact": "-0.022004860720170948",
      "effective_fee": "0.022880000000000000",
      "osmosis_route_data": {
        "routes": [
          {
            "pools": [
              {
                "id": 1,
                "type": 0,
                "spread_factor": "0.002000000000000000",
                "token_out_denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
                "taker_fee": "0.008000000000000000",
                "liquidity_cap": "1164149"
              },
              {
                "id": 2715,
                "type": 0,
                "spread_factor": "0.020000000000000000",
                "token_out_denom": "ibc/BC26A7A805ECD6822719472BCB7842A48EF09DF206182F8F259B2593EB5D23FB",
                "taker_fee": "0.015000000000000000",
                "liquidity_cap": "524503"
              }
            ],
            "has_cw_pool": false,
            "out_amount": "10258",
            "in_amount": "100000"
          }
        ],
        "liquidity_cap": "1688652",
        "liquidity_cap_overflow": false
      }
    },
    "outbound_legs": [],
    "outbound_supports_pfm": false,
    "execution": {
      "ibc_receiver": "",
      "recover_address": "",
      "min_output_amount": "10258",
      "uses_wasm": false,
      "description": "Same-chain swap on osmosis-1"
    }
  }
}
```

#### IBC Transfer from the Broker Chain

This can occure if the source chain is a broker chain(Osmosis in this example) and the destination chain is not a broker chain. It will return a single route to the destination chain. Example:

```json
{
  "success": true,
  "error_message": "",
  "broker_swap": {
    "path": [
      "osmosis-1",
      "cosmoshub-4"
    ],
    "inbound_leg": null,
    "swap": {
      "broker": "osmosis-sqs",
      "token_in": {
        "chain_denom": "uosmo",
        "base_denom": "uosmo",
        "origin_chain": "osmosis-1",
        "is_native": true
      },
      "token_out": {
        "chain_denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
        "base_denom": "uatom",
        "origin_chain": "cosmoshub-4",
        "is_native": false
      },
      "amount_in": "100000",
      "amount_out": "2188",
      "price_impact": "-0.000196369579903503",
      "effective_fee": "0.008000000000000000",
      "osmosis_route_data": {
        "routes": [
          {
            "pools": [
              {
                "id": 1400,
                "type": 2,
                "spread_factor": "0.000000000000000000",
                "token_out_denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
                "taker_fee": "0.008000000000000000",
                "liquidity_cap": "4304"
              }
            ],
            "has_cw_pool": false,
            "out_amount": "2188",
            "in_amount": "100000"
          }
        ],
        "liquidity_cap": "4304",
        "liquidity_cap_overflow": false
      }
    },
    "outbound_legs": [
      {
        "from_chain": "osmosis-1",
        "to_chain": "cosmoshub-4",
        "channel": "channel-0",
        "port": "transfer",
        "token": {
          "chain_denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
          "base_denom": "uatom",
          "origin_chain": "cosmoshub-4",
          "is_native": false
        },
        "amount": "2188"
      }
    ],
    "outbound_supports_pfm": true,
    "execution": {
      "memo": "",
      "ibc_receiver": "",
      "recover_address": "",
      "min_output_amount": "2188",
      "uses_wasm": false,
      "description": "Swap on osmosis-1 then IBC to cosmoshub-4"
    }
  }
}
```

#### Swam and IBC Transfer

This is a route that nvlolves swapping on the Broket chain and sending the assets to some other chain. In case
the query made to the pathfinder was with smart_route set to true the response will look like this:

```json
{
  "success": true,
  "error_message": "",
  "broker_swap": {
    "path": [
      "osmosis-1",
      "noble-1"
    ],
    "inbound_legs": [],
    "swap": {
      "broker": "osmosis-sqs",
      "token_in": {
        "chain_denom": "ibc/BC26A7A805ECD6822719472BCB7842A48EF09DF206182F8F259B2593EB5D23FB",
        "base_denom": "uatone",
        "origin_chain": "atomone-1",
        "is_native": false
      },
      "token_out": {
        "chain_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
        "base_denom": "uusdc",
        "origin_chain": "noble-1",
        "is_native": false
      },
      "amount_in": "5000000",
      "amount_out": "1577156",
      "price_impact": "-0.010143078324373824",
      "effective_fee": "0.009975000000000000",
      "osmosis_route_data": {
        "routes": [
          {
            "pools": [
              {
                "id": 2648,
                "type": 2,
                "spread_factor": "0.010000000000000000",
                "token_out_denom": "factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/alloyed/allBTC",
                "taker_fee": "0.005000000000000000",
                "liquidity_cap": "37685"
              },
              {
                "id": 1943,
                "type": 2,
                "spread_factor": "0.000100000000000000",
                "token_out_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
                "taker_fee": "0.005000000000000000",
                "liquidity_cap": "949322"
              }
            ],
            "has_cw_pool": false,
            "out_amount": "1577156",
            "in_amount": "5000000"
          }
        ],
        "liquidity_cap": "987007",
        "liquidity_cap_overflow": false
      }
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
        "amount": "1577156"
      }
    ],
    "outbound_supports_pfm": true,
    "execution": {
      "smart_contract_data": {
        "contract": "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
        "msg": {
          "swap_and_action": {
            "user_swap": {
              "swap_exact_asset_in": {
                "swap_venue_name": "osmosis-poolmanager",
                "operations": [
                  {
                    "pool": "2648",
                    "denom_in": "ibc/BC26A7A805ECD6822719472BCB7842A48EF09DF206182F8F259B2593EB5D23FB",
                    "denom_out": "factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/alloyed/allBTC"
                  },
                  {
                    "pool": "1943",
                    "denom_in": "factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/alloyed/allBTC",
                    "denom_out": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
                  }
                ]
              }
            },
            "min_asset": {
              "native": {
                "amount": "1561384",
                "denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
              }
            },
            "timeout_timestamp": "1770029170182608114",
            "post_swap_action": {
              "ibc_transfer": {
                "ibc_info": {
                  "memo": "",
                  "receiver": "noble1zjqm4lngspfqkp68psuv4suxwjfxftkenx9k4n",
                  "recover_address": "osmo1zjqm4lngspfqkp68psuv4suxwjfxftken7rwm0",
                  "source_channel": "channel-750"
                }
              }
            },
            "affiliates": []
          }
        }
      },
      "recover_address": "",
      "min_output_amount": "1561384",
      "uses_wasm": true,
      "description": "Smart contract swap on osmosis-1 then IBC to noble-1"
    }
  }
}
```

Under the execution field you will find the data required to execute the transaction. This is the data
required to execute the transaction by using smart_contract_data. You can then pass this info to the wallet on
the wallet to execute the transaction. The con for this is that smart route queries the broker interface
( Osmosis SQS in this example ) using singe_route parameter. What this means is the trade route might not the
most price efficient route. If you want to get the best possible price with the least slippage go for the
manual route.

#### Swap + Multi-hop Route

This is a route that involves sending asset to the Broker Chain and making a swap. From there the asset that has been traded is then being sent to the destination chain. This usually happens when you want to send tokens from chain A, and want to recieve another token on chain B. But to receive it you need to swap it on Osmosis for example. This requires a carefuly execution of multiple transactions or one carefully planned out Wasm smart contract exectuion. Example:

```json
{
  "success": true,
  "error_message": "",
  "broker_swap": {
    "path": [
      "cosmoshub-4",
      "osmosis-1",
      "juno-1"
    ],
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
      "effective_fee": "0.001500000000000000",
      "osmosis_route_data": {
        "routes": [
          {
            "pools": [
              {
                "id": 1282,
                "type": 2,
                "spread_factor": "0.000500000000000000",
                "token_out_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
                "taker_fee": "0.001500000000000000",
                "liquidity_cap": "19171"
              }
            ],
            "has_cw_pool": false,
            "out_amount": "9680597",
            "in_amount": "5000000"
          }
        ],
        "liquidity_cap": "19171",
        "liquidity_cap_overflow": false
      }
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
      },
      {
        "from_chain": "noble-1",
        "to_chain": "juno-1",
        "channel": "channel-3",
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
    "outbound_supports_pfm": true,
    "execution": {
      "memo": "{\"wasm\":{\"contract\":\"osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u\",\"msg\":{\"swap_and_action\":{\"user_swap\":{\"swap_exact_asset_in\":{\"swap_venue_name\":\"osmosis-poolmanager\",\"operations\":[{\"pool\":\"1282\",\"denom_in\":\"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2\",\"denom_out\":\"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4\"}]}},\"min_asset\":{\"native\":{\"amount\":\"9583791\",\"denom\":\"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4\"}},\"timeout_timestamp\":1770033664299672923,\"post_swap_action\":{\"ibc_transfer\":{\"ibc_info\":{\"memo\":\"{\\\"forward\\\":{\\\"channel\\\":\\\"channel-3\\\",\\\"port\\\":\\\"transfer\\\",\\\"receiver\\\":\\\"juno1zjqm4lngspfqkp68psuv4suxwjfxftkedhn92p\\\",\\\"retries\\\":2,\\\"timeout\\\":1770033664299672706}}\",\"receiver\":\"pfm\",\"recover_address\":\"osmo1zjqm4lngspfqkp68psuv4suxwjfxftken7rwm0\",\"source_channel\":\"channel-750\"}}},\"affiliates\":[]}}}}",
      "ibc_receiver": "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
      "recover_address": "osmo1zjqm4lngspfqkp68psuv4suxwjfxftken7rwm0",
      "min_output_amount": "9583791",
      "uses_wasm": true,
      "description": "IBC transfer with swap on osmosis-1 and forward via 2 hops to juno-1"
    }
  }
}
```

This is probably the most complicated one but the most common one. It involves a swap on the Broker Chain and then a multi-hop route to the destination chain.
In this example we initiate transfer from the Cosmos Hub by selecting Atom and sending an intent that we want to receive USDC on the Noble Chain.
For this to occure we first need to send the Atom to the Osmosis Broker Chain. This is done by sending an IBC transfer to the Osmosis. From there we can execute the swap from ATOM to USDC. Then this is sent to the Noble Chain.

If you plan to execute this transaction as fast as possible you can use the `memo` from the `execution` field
to execute the transaction as fast as possible. This is the data required to execute the transaction by using
the Skip smart sontracts. The Spectra Hub and the Pathfinder do leverage this Skip smart contract but in case
something goes wrong it should be able to recover to the `recover_address`. If something really goes wrong
with the smart contract you might need to contact the Skip which is behind the smart contract.

#### Multi-hop Route

It is a bit rare but it might happen, some chains might use other chains for intermediate hops ( to my
knowledge I think only Sei does this via Osmosis for some tokens ). Another reason that might be good would
be to send one token that has usege on chain A and chain B but instead of making 2 transactions we use Packet Forwarding Middleware. Example:

```json
{
  "success": true,
  "error_message": "",
  "indirect": {
    "path": [
      "osmosis-1",
      "noble-1",
      "juno-1"
    ],
    "legs": [
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
        "amount": "1000000"
      },
      {
        "from_chain": "noble-1",
        "to_chain": "juno-1",
        "channel": "channel-3",
        "port": "transfer",
        "token": {
          "chain_denom": "uusdc",
          "base_denom": "uusdc",
          "origin_chain": "noble-1",
          "is_native": true
        },
        "amount": "1000000"
      }
    ],
    "supports_pfm": true,
    "pfm_start_chain": "osmosis-1",
    "pfm_memo": "{\"forward\":{\"receiver\":\"juno1zjqm4lngspfqkp68psuv4suxwjfxftkedhn92p\",\"port\":\"transfer\",\"channel\":\"channel-3\"}}"
  }
}
```

The clear example would be stablecoin routing. Osmosis to Juno for example you would need to send the USDC to Noble Chain first, then from Noble to Juno. This can all be done in one go via PFM module (if it is supported).

### GetChainInfo

This method is used to get the information about a chain. It will return the chain ID, chain name, and the
basic information about the chain.

#### GetChainInfo - Request

The request should contain the following fields:

- chain_id: The chain ID of the chain.
- show_symbols: Whether to show the symbols of the tokens on the chain(optional, default is false).

#### GetChainInfo - Response

Example for the Atom One chain:

```json
{
  "chain_info": {
    "chain_id": "atomone-1",
    "chain_name": "Atom One",
    "has_pfm": false,
    "is_broker": false,
    "routes": [
      {
        "to_chain": "Osmosis",
        "to_chain_id": "osmosis-1",
        "connection_id": "connection-2",
        "channel_id": "channel-2",
        "port_id": "transfer",
        "allowed_tokens": {
          "uatone": {
            "chain_denom": "uatone",
            "ibc_denom": "ibc/BC26A7A805ECD6822719472BCB7842A48EF09DF206182F8F259B2593EB5D23FB",
            "base_denom": "uatone",
            "origin_chain": "atomone-1",
            "decimals": 6,
            "symbol": "ATONE"
          },
          "uphoton": {
            "chain_denom": "uphoton",
            "ibc_denom": "ibc/D6E02C5AE8A37FC2E3AB1FC8AC168878ADB870549383DFFEA9FD020C234520A7",
            "base_denom": "uphoton",
            "origin_chain": "atomone-1",
            "decimals": 6,
            "symbol": "PHOTON"
          }
        }
      },
      {
        "to_chain": "Stargaze",
        "to_chain_id": "stargaze-1",
        "connection_id": "connection-7",
        "channel_id": "channel-3",
        "port_id": "transfer",
        "allowed_tokens": {
          "uatone": {
            "chain_denom": "uatone",
            "ibc_denom": "ibc/1CB8A5D27AD8BEBAFF6C810E637157E07703662AA084E68330E388F77E27244D",
            "base_denom": "uatone",
            "origin_chain": "atomone-1",
            "decimals": 6,
            "symbol": "ATONE"
          }
        }
      }
    ]
  }
}
```

### GetTokenDenoms

This method is used to get the information about a token. It will return the token information for the given chain.

#### GetTokenDenoms - Request

The request should contain the following fields:

- base_denom: The base denom of the token.
- origin_chain: The chain ID of the chain where the token is native.
- on_chain_id: The chain ID of the chain to get the token information for(optional, default is to get all of the denoms accross the Pathfinder supported chains).

#### GetTokenDenoms - Response

Example for using with this request parameters:

```json
{
  "baseDenom": "uatone",
  "originChain": "atomone-1",
  "onChainId": "osmosis-1"
}
```

Response:

```json
{
  "found": true,
  "base_denom": "uatone",
  "origin_chain": "atomone-1",
  "denoms": [
    {
      "chain_id": "osmosis-1",
      "chain_name": "Osmosis",
      "denom": "ibc/BC26A7A805ECD6822719472BCB7842A48EF09DF206182F8F259B2593EB5D23FB",
      "is_native": false
    }
  ]
}
```

This response will return the token information for the given chain. It will return the chain ID, chain name, and the
token information for the given chain.

### GetChainTokens

This method is used to get the information about a chain. It will return the chain ID, chain name, and the
basic information about the chain.

#### GetChainTokens - Request

The request should contain the following fields:

- chain_id: The chain ID of the chain to get the token information for.

#### GetChainTokens - Response

Example response for Cosmoshub-4:

```json
{
  "chain_id": "cosmoshub-4",
  "chain_name": "Cosmos Hub",
  "native_tokens": [
    {
      "denom": "uatom",
      "symbol": "ATOM",
      "base_denom": "uatom",
      "origin_chain": "cosmoshub-4",
      "decimals": 6,
      "is_native": true
    }
  ],
  "ibc_tokens": [
    {
      "denom": "ibc/F663521BF1836B00F5F177680F74BFB9A8B5654A694D0D2BC249E03CF2509013",
      "symbol": "USDC",
      "base_denom": "uusdc",
      "origin_chain": "noble-1",
      "decimals": 6,
      "is_native": false
    },
    {
      "denom": "ibc/99B00614DDBE6189AA03B77066FF8EB3F93680BD790C43CF56096B7F23542015",
      "symbol": "WBTC",
      "base_denom": "factory/osmo1z0qrq605sjgcqpylfl4aa6s90x738j7m58wyatt0tdzflg2ha26q67k743/wbtc",
      "origin_chain": "osmosis-1",
      "decimals": 8,
      "is_native": false
    }
  ]
}
```

### GetPathfinderSupportedChains

This method is used to get the list of all supported chains by the Pathfinder.

#### GetPathfinderSupportedChains - Request

The request should contain the following fields:

- None

#### GetPathfinderSupportedChains - Response

Example response:

```json
{
  "chain_ids": [
    "juno-1",
    "noble-1",
    "osmosis-1",
    "stargaze-1",
    "symphony-1",
    "atomone-1",
    "cosmoshub-4"
  ]
}
```

### LookupDenom

This method is used to get the information about a token. It will return the token information for the given chain.

#### LookupDenom - Request

The request should contain the following fields:

- chain_id: The chain ID of the chain to get the token information for.
- denom: The denom of the token to get the information for. Can also use IBC denoms and native chain denom + @ origin_chain to get the information for the token on the origin chain.

#### LookupDenom - Response

Example response for the ATONE token on stargaze-1:

```json
{
  "found": true,
  "chain_denom": "ibc/1CB8A5D27AD8BEBAFF6C810E637157E07703662AA084E68330E388F77E27244D",
  "base_denom": "uatone",
  "origin_chain": "atomone-1",
  "is_native": false,
  "ibc_path": "transfer/channel-448",
  "available_on": [
    {
      "chain_id": "osmosis-1",
      "chain_name": "Osmosis",
      "denom": "ibc/BC26A7A805ECD6822719472BCB7842A48EF09DF206182F8F259B2593EB5D23FB",
      "is_native": false
    },
    {
      "chain_id": "stargaze-1",
      "chain_name": "Stargaze",
      "denom": "ibc/1CB8A5D27AD8BEBAFF6C810E637157E07703662AA084E68330E388F77E27244D",
      "is_native": false
    },
    {
      "chain_id": "atomone-1",
      "chain_name": "Atom One",
      "denom": "uatone",
      "is_native": true
    }
  ]
}
```

It shows more wide information about the token and the available chains where the token is available. This will stay like this for now but it might change later.

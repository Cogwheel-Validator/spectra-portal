# Config Manager

The Config Manager is responsible for transforming human-readable chain configurations into generated configurations used by the solver backend and frontend client applications.

## Data Flow

The human readable config should strictly conist of TOML files. The generated files can be of type TOML or JSON if desired. The default output is `generated` directory.


## Writing Chain Configs

Chain configs are written in TOML format and placed in the `chain_configs/` directory.

### Required Fields

```toml
[chain]
name = "Chain Name"           # Human-readable name
id = "chain-1"                # Chain ID
type = "cosmos"               # Chain type (only "cosmos" supported)
registry = "chainname"        # Directory in cosmos/chain-registry
explorer_url = "https://..."  # Block explorer URL
slip44 = 118                  # SLIP-44 coin type
bech32_prefix = "prefix"      # Address prefix

# At least one RPC endpoint
[[chain.rpcs]]
url = "https://rpc.example.com"
provider = "Provider Name"    # Optional

# At least one REST endpoint
[[chain.rest]]
url = "https://api.example.com"
provider = "Provider Name"    # Optional

# At least one native token
[[token]]
denom = "utoken"
name = "Token Name"
symbol = "TOKEN"
exponent = 6
icon = "https://..."

# Multihop example
[[token]]
denom = "ibc/COMPUTED_HASH"  # Compute: transfer/channel-2/ibc/987C17...
name = "Token Name"
symbol = "TOKEN"
exponent = 6
icon = "https://..."
origin_chain = "chain-1"
origin_denom = "utoken"
allowed_destinations = ["osmosis-1"]  # Can only send to Osmosis 
```

### Optional Fields

```toml
[chain]
# For DEX chains that can perform swaps
is_broker = true
broker_id = "osmosis-sqs"

# Packet Forward Middleware support
has_pfm = true

[[token]]
# CoinGecko ID for price data
coingecko_id = "token-id"

# Restrict token to specific destination chains
allowed_destinations = ["osmosis-1", "cosmoshub-4"]
```

### Example: Complete Chain Config

See `chain_configs/osmosis.toml` for a complete example.

## Running the Generator

### Using the CLI

```bash
# Generate both solver and client configs
go run ./config_manager/cmd/generate \
  -input ./chain_configs \
  -solver-output ./generated/solver_config.toml \
  -client-output ./generated/client_config.json

# Validate only (no output)
go run ./config_manager/cmd/generate \
  -input ./chain_configs \
  -validate-only

# Skip network checks (faster, for development)
go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --skip-network \
  --solver-output ./generated/solver_config.toml

# If you need output in a different format, you can use the following flags:
go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --solver-output ./generated/solver_config.toml \
  --client-output ./generated/client_config.json \
  --solver-format toml \
  --client-format json

# If you need to use a cached registry data, you can use the following flag:

go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --registry-cache ./cache/registry.json \
  --use-cache

# If you need to skip network validation, you can use the following flag:


go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --skip-network

# If you need to validate only, you can use the following flag:


go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --validate-only

```

### Programmatic Usage

```go
import (
    "github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/pipeline"
)

config := pipeline.GeneratorConfig{
    InputDir:         "./chain_configs",
    SolverOutputPath: "./generated/solver_config.toml",
    ClientOutputPath: "./generated/client_config.json",
}

generator := pipeline.NewGenerator(config)
result, err := generator.Generate()
```

## Generated Configs

### Solver Config (Backend)

The solver config is used by the routing solver to build the route index:

```go
import (
    "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/config"
    "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/router"
)

loader := config.NewChainConfigLoader()

// Load and initialize the solver
solver, err := loader.InitializeSolver(
    "./generated/solver_config.toml",
    brokerClients, // map[string]router.BrokerClient
)

// Use the solver
response := solver.Solve(routeRequest)
```

### Client Config (Frontend)

The client config is a JSON file designed for frontend consumption:

```typescript
interface ClientConfig {
  version: string;
  generated_at: string;
  chains: ClientChain[];
  all_tokens: ClientTokenSummary[];
}

// Load in your frontend app
const config = await fetch('/config/client_config.json').then(r => r.json());

// Access chain data
config.chains.forEach(chain => {
  console.log(chain.name, chain.native_tokens, chain.connected_chains);
});

// Quick token lookup
const atomToken = config.all_tokens.find(t => t.symbol === 'ATOM');
console.log('ATOM available on:', atomToken.available_on);
```

## Data Flow

1. **Input**: Developer writes `chain_configs/mychain.toml`
2. **Validation**: Input config is validated for required fields and types
3. **Registry Fetch**: IBC channel data is fetched from cosmos/chain-registry
4. **Endpoint Verification**: RPC/REST endpoints are health-checked
5. **Enrichment**: Input config is enriched with IBC routes and token mappings
6. **Conversion**: Enriched config is converted to solver and client formats
7. **Output**: Generated configs are written to disk

## Adding a New Chain

1. Create a new file in `chain_configs/` (e.g., `mychain.toml`)
2. Fill in the required fields based on your chain's specifications
3. Run the generator to validate and generate configs
4. The chain will be automatically included in IBC routing

## Troubleshooting

### "No IBC connections found"

Ensure the `registry` field matches the exact directory name in the [cosmos/chain-registry](https://github.com/cosmos/chain-registry/tree/master/_IBC). Also make sure that the registy is filled with the correct data.

### "Endpoint not reachable"

Check that your RPC/REST URLs are correct and accessible. Use `--skip-network` for development.

### "Duplicate chain ID"

Each chain config must have a unique `chain.id`. Check for duplicates in your config files.


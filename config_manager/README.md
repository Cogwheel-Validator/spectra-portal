# Config Manager

The Config Manager is responsible for transforming human-readable chain configurations into generated configurations used by the pathfinder backend and frontend client applications.

## Configuration Flow

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
# Generate both pathfinder and client configs
go run ./config_manager/cmd/generate \
  -input ./chain_configs \
  -pathfinder-output ./generated/pathfinder_config.toml \
  -client-output ./generated/client_config.json
```

```bash
# Validate only (no output)
go run ./config_manager/cmd/generate \
  -input ./chain_configs \
  -validate-only
```

```bash
# Skip network checks (faster, for development)
go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --skip-network \
  --pathfinder-output ./generated/pathfinder_config.toml
```

```bash
# If you need output in a different format, you can use the following flags:
go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --pathfinder-output ./generated/pathfinder_config.toml \
  --client-output ./generated/client_config.json \
  --pathfinder-format toml \
  --client-format json
```

```bash
# If you need to use a local registry file, you can use the following flag and set the use-local-data flag to true:

go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --local-registry-cache ./ibc_registry \
  --local-keplr-cache ./keplr-registry \
  --use-local-data

# If you need to skip network validation, you can use the following flag:
```bash

go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --skip-network
```

```bash
# If you need to validate only, you can use the following flag:


go run ./config_manager/cmd/generate \
  --input ./chain_configs \
  --validate-only

```

Or if you have `make` installed, you can use the following command:

```bash
# for running the generator with local registry and keplr registry
make generate-config-l
```

Or something like this can work if you need fresh registries:

```bash
# for running the generator with fresh registries
make generate-config
```

## Data Flow

1. **Input**: Developer writes `chain_configs/mychain.toml`
2. **Validation**: Input config is validated for required fields and types
3. **Registry Fetch**: IBC channel data is fetched from cosmos/chain-registry and keplr registry from chainapsis github repository
4. **Endpoint Verification**: RPC/REST endpoints are health-checked
5. **Enrichment**: Input config is enriched with IBC routes and token mappings
6. **Conversion**: Enriched config is converted to pathfinder and client formats
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

Check that your RPC/REST URLs are correct and accessible. Use `--skip-network` for development when you generate config files.

### "Duplicate chain ID"

Each chain config must have a unique `chain.id`. Check for duplicates in your config files.

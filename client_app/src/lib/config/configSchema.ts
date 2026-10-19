import { z } from "zod";
import type { ClientConfig } from "@/components/modules/tomlTypes";

const endpointSchema = z.object({
    url: z.string(),
    provider: z.string().optional(),
});

const tokenSchema = z.object({
    denom: z.string(),
    name: z.string(),
    symbol: z.string(),
    decimals: z.number(),
    icon: z.string(),
    origin_chain: z.string(),
    origin_chain_name: z.string().optional(),
    coingecko_id: z.string().optional(),
    is_native: z.boolean(),
    base_denom: z.string().optional(),
});

const tokenSummarySchema = z.object({
    base_denom: z.string(),
    symbol: z.string(),
    name: z.string(),
    icon: z.string(),
    origin_chain: z.string(),
    origin_chain_name: z.string(),
    available_on: z.array(z.string()),
    coingecko_id: z.string().optional(),
});

const connectedChainSchema = z.object({
    id: z.string(),
    name: z.string(),
    logo: z.string().optional(),
    sendable_tokens: z.array(z.string()),
});

const keplrCurrencySchema = z.looseObject({
    coin_denom: z.string(),
    coin_minimal_denom: z.string(),
    coin_decimals: z.number(),
});

// Passthrough keys are copied by reference, not rebuilt, so any nested object
// among them would keep smol-toml's null prototype. Nested objects must
// therefore be declared (see stake_currency); primitives are safe.
// Loose so extra keys the generator emits (e.g. coin_image_url) still reach
// Keplr's suggestChain instead of being stripped.
const keplrChainConfigSchema = z.looseObject({
    rpc: z.string(),
    rest: z.string(),
    chain_id: z.string(),
    chain_name: z.string(),
    chain_symbol_image_url: z.string(),
    bip44: z.looseObject({ coin_type: z.number() }),
    bech32_config: z.looseObject({
        bech32_prefix_acc_addr: z.string(),
        bech32_prefix_acc_pub: z.string(),
        bech32_prefix_val_addr: z.string(),
        bech32_prefix_val_pub: z.string(),
        bech32_prefix_cons_addr: z.string(),
        bech32_prefix_cons_pub: z.string(),
    }),
    currencies: z.array(keplrCurrencySchema),
    stake_currency: keplrCurrencySchema.optional(),
    fee_currencies: z.array(
        keplrCurrencySchema.extend({
            gas_price_step: z.looseObject({
                low: z.number(),
                average: z.number(),
                high: z.number(),
            }),
        }),
    ),
});

const chainSchema = z.object({
    name: z.string(),
    id: z.string(),
    bech32_prefix: z.string(),
    slip44: z.number(),
    account_type: z.enum(["cosmos", "ethereum", "ethermint"]),
    evm_chain_id: z.number().optional(),
    explorer_details: z.object({
        base_url: z.string(),
        transaction_path: z.string(),
        account_path: z.string(),
    }),
    cosmos_sdk_version: z.string(),
    chain_logo: z.string().optional(),
    is_dex: z.boolean(),
    rpc_endpoints: z.array(endpointSchema),
    rest_endpoints: z.array(endpointSchema),
    native_tokens: z.array(tokenSchema),
    ibc_tokens: z.array(tokenSchema),
    connected_chains: z.array(connectedChainSchema),
    keplr_chain_config: keplrChainConfigSchema,
});

/**
 * Validates and rebuilds the parsed config. Zod returns fresh plain objects,
 * which also matters for Next.js: smol-toml produces null-prototype objects
 * that cannot be passed from Server to Client Components.
 */
export const clientConfigSchema = z.object({
    version: z.string(),
    generated_at: z.string(),
    chains: z.array(chainSchema),
    all_tokens: z.array(tokenSummarySchema),
}) satisfies z.ZodType<ClientConfig>;

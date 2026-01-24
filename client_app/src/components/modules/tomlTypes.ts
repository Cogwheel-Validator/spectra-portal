export type ClientConfig = {
    version: string;
    generated_at: string;
    chains: ClientChain[];
    all_tokens: ClientTokenSummary[];
};

export type ClientChain = {
    name: string;
    id: string;
    bech32_prefix: string;
    slip44: number;
    explorer_details: ExplorerDetails;
    cosmos_sdk_version: string;
    chain_logo?: string;
    is_dex: boolean;
    rpc_endpoints: ClientEndpoint[];
    rest_endpoints: ClientEndpoint[];
    native_tokens: ClientToken[];
    ibc_tokens: ClientToken[];
    connected_chains: ConnectedChainInfo[];
    keplr_chain_config: KeplrChainConfig;
};

export type ExplorerDetails = {
    base_url: string;
    transaction_path: string;
    account_path: string;
};

export type ClientEndpoint = {
    url: string;
    provider?: string;
};

export type ClientToken = {
    denom: string;
    name: string;
    symbol: string;
    decimals: number;
    icon: string;
    origin_chain: string;
    origin_chain_name?: string;
    coingecko_id?: string;
    is_native: boolean;
    base_denom?: string;
};

export type ConnectedChainInfo = {
    id: string;
    name: string;
    logo?: string;
    sendable_tokens: string[];
};

export type ClientTokenSummary = {
    base_denom: string;
    symbol: string;
    name: string;
    icon: string;
    origin_chain: string;
    origin_chain_name: string;
    available_on: string[];
    coingecko_id?: string;
};

export type KeplrChainConfig = {
    rpc: string;
    rest: string;
    chain_id: string;
    chain_name: string;
    chain_symbol_image_url: string;
    bip44: {
        coin_type: number;
    };
    bech32_config: {
        bech32_prefix_acc_addr: string;
        bech32_prefix_acc_pub: string;
        bech32_prefix_val_addr: string;
        bech32_prefix_val_pub: string;
        bech32_prefix_cons_addr: string;
        bech32_prefix_cons_pub: string;
    };
    currencies: {
        coin_denom: string;
        coin_minimal_denom: string;
        coin_decimals: number;
    }[];
    fee_currencies: {
        coin_denom: string;
        coin_minimal_denom: string;
        coin_decimals: number;
        gas_price_step: {
            low: number;
            average: number;
            high: number;
        };
    }[];
};

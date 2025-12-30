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
    explorer_url: string;
    chain_logo?: string;
    is_dex: boolean;
    rpc_endpoints: ClientEndpoint[];
    rest_endpoints: ClientEndpoint[];
    native_tokens: ClientToken[];
    ibc_tokens: ClientToken[];
    connected_chains: ConnectedChainInfo[];
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

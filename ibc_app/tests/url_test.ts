import { expect, test } from "bun:test";
import type { ClientChain, KeplrChainConfig } from "@/components/modules/tomlTypes";
import { makeUrlForTransactionByEvents } from "@/lib/apiQueries/fetchApiData";

//rpc_endpoints, native_tokens, ibc_tokens, connected_chains, keplr_chain_config

const emptyKeplrConf: KeplrChainConfig = {
    rpc: "",
    rest: "",
    chain_id: "",
    chain_name: "",
    chain_symbol_image_url: "",
    bip44: {
        coin_type: 0
    },
    bech32_config: {
        bech32_prefix_acc_addr: "",
        bech32_prefix_acc_pub: "",
        bech32_prefix_val_addr: "",
        bech32_prefix_val_pub: "",
        bech32_prefix_cons_addr: "",
        bech32_prefix_cons_pub: ""
    },
    currencies: [],
    fee_currencies: []
};

const v050ChainConfig: ClientChain = {
    id: "osmosis-1",
    name: "Osmosis",
    bech32_prefix: "osmo",
    slip44: 118,
    explorer_details: {
        base_url: "https://mintscan.io",
        account_path: "osmosis/address/{address}",
        transaction_path: "osmosis/txs/{tx_hash}",
    },
    cosmos_sdk_version: "v0.50.14-v30-osmo",
    chain_logo: "/icons/osmosis/logo.png",
    is_dex: true,
    rest_endpoints: [{
        url: "https://osmosis-api.cogwheel.zone",
        provider: "Cogwheel",
    }],
    rpc_endpoints: [],
    native_tokens: [],
    ibc_tokens: [],
    connected_chains: [],
    keplr_chain_config: emptyKeplrConf,
};

const v047ChainConfig: ClientChain = {
    id: "atomone-1",
    name: "Atom One",
    bech32_prefix: "atone",
    slip44: 118,
    explorer_details: {
        base_url: "https://thespectra.io",
        account_path: "atomone/address/{address}",
        transaction_path: "atomone/transactions/{tx_hash}",
    },
    cosmos_sdk_version: "v0.47.11",
    is_dex: false,
    rpc_endpoints: [],
    rest_endpoints: [{
        url: "https://atomone-api.cogwheel.zone",
        provider: "Cogwheel",
    }],
    native_tokens: [],
    ibc_tokens: [],
    connected_chains: [],
    keplr_chain_config: emptyKeplrConf,
};

test("makeUrlForTransactionByEventsV050", () => {
    const apiUrl = v050ChainConfig.rest_endpoints[0].url;
    const url = makeUrlForTransactionByEvents(
        apiUrl,
        v050ChainConfig,
        {
            queries: ["fungible_token_packet.sender='atone16fxth82zn0zxr9mc2k6g9mc6fmv2ysf9kephw9'", "fungible_token_packet.amount='851000000'"],
            limit: 1,
        }
    )
    console.log(url);
    expect(url).toBe("https://osmosis-api.cogwheel.zone/cosmos/tx/v1beta1/txs?order_by=ORDER_BY_UNSPECIFIED&limit=1&query=fungible_token_packet.sender%3D'atone16fxth82zn0zxr9mc2k6g9mc6fmv2ysf9kephw9'%20AND%20fungible_token_packet.amount%3D'851000000'");
});

test("makeUrlForTransactionByEventsV047", () => {
    const apiUrl = v047ChainConfig.rest_endpoints[0].url;
    const url = makeUrlForTransactionByEvents(
        apiUrl,
        v047ChainConfig,
        {
            queries: ["fungible_token_packet.sender='atone16fxth82zn0zxr9mc2k6g9mc6fmv2ysf9kephw9'", "fungible_token_packet.amount='851000000'"],
            limit: 1,
        }
    )
    console.log(url);
    expect(url).toBe("https://atomone-api.cogwheel.zone/cosmos/tx/v1beta1/txs?events=fungible_token_packet.sender%3D'atone16fxth82zn0zxr9mc2k6g9mc6fmv2ysf9kephw9'&events=fungible_token_packet.amount%3D'851000000'&order_by=ORDER_BY_UNSPECIFIED&limit=1");
});
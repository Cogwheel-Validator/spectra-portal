import { join } from "node:path";
import * as TOML from "smol-toml";
import type {
    ClientChain,
    ClientConfig,
    ClientToken,
    ClientTokenSummary,
    ConnectedChainInfo,
} from "@/components/modules/tomlTypes";

/**
 * Loads the client config at build time.
 * This file is evaluated at build time, so the config is compiled into the bundle.
 *
 * The config file is located one level above the Next.js app in generated/client_config.json
 * or in the public directory as config.json (copied during build)
 *
 * @param format - The format of the config file: 'toml' or 'json'. If not provided, will auto-detect from file extension.
 */
export async function LoadConfig(format?: string): Promise<FullClientConfig> {
    if (format && format !== "toml" && format !== "json") {
        throw new Error("Invalid format. Must be 'toml' or 'json'");
    }

    // Try both formats if format is not specified
    const formatsToTry = format ? [format] : ["toml", "json"];

    for (const fmt of formatsToTry) {
        const possiblePaths = [
            // Path relative to Next.js app root (one level up)
            join(process.cwd(), "..", "generated", `client_config.${fmt}`),
            // Absolute path fallback (if process.cwd() is the project root)
            join(process.cwd(), "generated", `client_config.${fmt}`),
            // Path like the config is placed in the root of the Next.js app (for Docker)
            join(process.cwd(), `client_config.${fmt}`),
        ];

        for (const configPath of possiblePaths) {
            const file = Bun.file(configPath);
            if (await file.exists()) {
                const text = await file.text();
                let parsedConfig: ClientConfig;

                if (fmt === "toml") {
                    parsedConfig = TOML.parse(text) as ClientConfig;
                } else {
                    parsedConfig = JSON.parse(text) as ClientConfig;
                }

                return new FullClientConfig(parsedConfig);
            }
        }
    }

    throw new Error(
        `Client config not found. Tried paths for formats: ${formatsToTry.join(", ")}\n` +
            `Please run 'make generate-config' to generate the config file, or ensure it's copied to the expected location.`,
    );
}

class FullClientConfig {
    private chains: Map<string, ClientChain> = new Map();
    private tokenSummaries: Map<string, ClientTokenSummary> = new Map();
    private chainLogos: Map<string, string | undefined> = new Map();
    public config: ClientConfig;

    constructor(config: ClientConfig) {
        this.chains = new Map(config.chains.map((chain) => [chain.id, chain]));
        this.tokenSummaries = new Map(config.all_tokens.map((token) => [token.base_denom, token]));
        this.chainLogos = new Map(config.chains.map((chain) => [chain.id, chain.chain_logo]));
        this.config = config;
    }

    public getChainById(chainId: string): ClientChain | undefined {
        return this.chains.get(chainId);
    }

    public getChainByName(chainName: string): ClientChain | undefined {
        return this.chains.values().find((chain) => chain.name === chainName);
    }

    public getTokensForChain(chainId: string): ClientToken[] {
        const chain = this.getChainById(chainId);
        if (!chain) return [];

        return [...chain.native_tokens, ...chain.ibc_tokens];
    }

    public getTokenBySymbol(chainId: string, symbol: string): ClientToken | undefined {
        const tokens = this.getTokensForChain(chainId);
        return tokens.find((token) => token.symbol === symbol);
    }

    public getConnectedChains(chainId: string): ConnectedChainInfo[] {
        const chain = this.getChainById(chainId);
        if (!chain) return [];

        return chain.connected_chains;
    }

    public getSendableTokens(fromChainId: string, toChainId: string): string[] {
        const fromChain = this.getChainById(fromChainId);
        if (!fromChain) return [];

        const connectedChain = fromChain.connected_chains.find((chain) => chain.id === toChainId);
        if (!connectedChain) return [];

        return connectedChain.sendable_tokens;
    }

    public getTokenLogo(denom: string): string | undefined {
        const tokenSummary = this.tokenSummaries.get(denom);
        if (!tokenSummary) return undefined;

        return tokenSummary.icon;
    }

    public getChainLogo(chainId: string): string | undefined {
        const chain = this.getChainById(chainId);
        if (!chain) return undefined;

        return chain.chain_logo;
    }
}

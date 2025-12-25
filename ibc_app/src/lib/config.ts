import { existsSync } from "node:fs";
import { join } from "node:path";
import type { ClientChain, ClientConfig, ClientToken, ClientTokenSummary, ConnectedChainInfo } from "@/components/modules/tomlTypes";

/**
 * Loads the client config at build time.
 * This file is evaluated at build time, so the config is compiled into the bundle.
 * 
 * The config file is located one level above the Next.js app in generated/client_config.json
 * or in the public directory as config.json (copied during build)
 */
export async function LoadConfig(format: string): Promise<FullClientConfig> {
  if (format !== "toml" && format !== "json") {
    throw new Error("Invalid format. Must be 'toml' or 'json'");
  }

  let possiblePaths: string[] = [];

  if (format === "toml") {
  // Try multiple paths to find the config
  possiblePaths = [
    // Path relative to Next.js app root (one level up)
    join(process.cwd(), "..", "generated", "client_config.toml"),
    // Absolute path fallback (if process.cwd() is the project root)
    join(process.cwd(), "generated", "client_config.toml"),
    // Path like the config is placed in the root of the Next.js app(for Docker)
    join(process.cwd(), "client_config.toml"),
  ];
  } else {
    possiblePaths = [
      join(process.cwd(), "..", "generated", "client_config.json"),
      join(process.cwd(), "generated", "client_config.json"),
      join(process.cwd(), "client_config.json"),
    ];
  }

  for (const configPath of possiblePaths) {
    if (existsSync(configPath)) {
        // bun should be able to import the config file directly
        const config: ClientConfig = await import(configPath);
        return new FullClientConfig(config);
    }
  }
  throw new Error(`Client config not found. Tried paths: ${possiblePaths.join(", ")}\nPlease run 'make generate-config' to generate the config file, or ensure it's copied to public/config.json`);
}

class FullClientConfig {
  private chains: Map<string, ClientChain> = new Map();
  private tokenSummaries: Map<string, ClientTokenSummary> = new Map();
  private chainLogos: Map<string, string | undefined> = new Map();

  constructor(config: ClientConfig) {
    this.chains = new Map(config.chains.map((chain) => [chain.id, chain]));
    this.tokenSummaries = new Map(config.all_tokens.map((token) => [token.base_denom, token]));
    this.chainLogos = new Map(config.chains.map((chain) => [chain.id, chain.chain_logo]));
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

    const connectedChain = fromChain.connected_chains.find(
      (chain) => chain.id === toChainId
    );
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

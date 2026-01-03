"use client";

import { Buffer } from "node:buffer";
import { type EncodeObject, type GeneratedType, Registry } from "@cosmjs/proto-signing";
import {
    AminoTypes,
    calculateFee,
    createDefaultAminoConverters,
    defaultRegistryTypes,
    GasPrice,
    SigningStargateClient,
    StargateClient,
    type StdFee,
} from "@cosmjs/stargate";
import { TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx";
import { createContext, type ReactNode, useCallback, useContext, useEffect, useState } from "react";
import type { ClientChain } from "@/components/modules/tomlTypes";
import { getRandomHealthyRpc } from "@/lib/apiQueries/featchHealthyEndpoint";
import { MsgSplitRouteSwapExactAmountIn, MsgSwapExactAmountIn } from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/tx";
import { type WalletConnectionState, WalletType } from "@/lib/wallets/walletProvider";
import { getWalletProviderAsync } from "@/lib/wallets/walletUtility";

// Polyfill Buffer for client-side usage
if (typeof window !== "undefined") {
    window.Buffer = window.Buffer || Buffer;
}

/**
 * Chain connection info stored per chain
 */
interface ChainConnection {
    chainId: string;
    address: string;
    chainConfig: ClientChain;
}

export interface TransactionFee {
    amount: Array<{ denom: string; amount: string }>;
    gas: string;
}

export interface TransactionResult {
    transactionHash: string;
    code: number;
    height?: number;
}

export interface TransactionOptions {
    fee?: TransactionFee | "auto";
    memo?: string;
    gasAdjustment?: number;
    chainConfig?: ClientChain;
}

/**
 * Simplified Wallet Context Interface - Only 6 core methods!
 */
interface WalletContextType {
    // State (read-only)
    isConnected: boolean;
    chains: ClientChain[];
    walletType: WalletType | null;
    
    // Chain queries
    getAddress: (chainId: string) => string | null;
    getChainConfig: (chainId: string) => ClientChain | null;
    isConnectedToChain: (chainId: string) => boolean;
    
    // Connection management
    connection: {
        connect: (chainConfigs: ClientChain[], walletType?: WalletType) => Promise<void>;
        disconnect: (chainIds?: string[]) => void;
        suggestChain: (chainConfigs: ClientChain[], walletType?: WalletType) => Promise<void>;
    };
    
    // Generic transaction method - handles ALL transaction types
    sendTransaction: (
        chainId: string,
        messages: EncodeObject[],
        options?: TransactionOptions
    ) => Promise<TransactionResult>;
}

/**
 * Get wallet provider based on wallet type
 */
const getWalletFromProvider = async (walletType: WalletType = WalletType.KEPLR) => {
    if (typeof window === "undefined") {
        throw new Error("Wallet is only available in browser environment");
    }
    return await getWalletProviderAsync(walletType);
};

const WalletContext = createContext<WalletContextType | undefined>(undefined);

export const useWallet = () => {
    const context = useContext(WalletContext);
    if (!context) {
        throw new Error("useWallet must be used within WalletProvider");
    }
    return context;
};

export const WalletProvider = ({ children }: { children: ReactNode }) => {
    // Multi-chain connection state
    const [connections, setConnections] = useState<Map<string, ChainConnection>>(new Map());
    const [walletType, setWalletType] = useState<WalletType | null>(null);

    // Derived state
    const isConnected = connections.size > 0;
    const chains = Array.from(connections.values()).map((conn) => conn.chainConfig);

    // Helper to get address for a specific chain
    const getAddress = useCallback((chainId: string): string | null => {
        return connections.get(chainId)?.address || null;
    }, [connections]);

    // Helper to get chain config for a specific chain
    const getChainConfig = useCallback((chainId: string): ClientChain | null => {
        return connections.get(chainId)?.chainConfig || null;
    }, [connections]);

    // Helper to check if connected to a specific chain
    const isConnectedToChain = useCallback((chainId: string): boolean => {
        return connections.has(chainId);
    }, [connections]);

    // Persist wallet connection state per chain
    const saveWalletState = (walletData: WalletConnectionState & { chainConfig: ClientChain }) => {
        if (typeof window !== "undefined") {
            const key = `spectra_ibc_wallet_${walletData.chainId}`;
            localStorage.setItem(key, JSON.stringify(walletData));
            localStorage.setItem("spectra_ibc_wallet_type", walletData.walletType);
        }
    };

    const loadAllWalletStates = useCallback(() => {
        if (typeof window === "undefined") return [];
        
        const savedStates: (WalletConnectionState & { chainConfig: ClientChain })[] = [];
        const savedWalletType = localStorage.getItem("spectra_ibc_wallet_type");
        
        if (!savedWalletType) return [];

        for (let i = 0; i < localStorage.length; i++) {
            const key = localStorage.key(i);
            if (key?.startsWith("spectra_ibc_wallet_") && key !== "spectra_ibc_wallet_type") {
                const saved = localStorage.getItem(key);
                if (saved) {
                    try {
                        savedStates.push(JSON.parse(saved));
                    } catch (error) {
                        console.error(`Failed to parse saved state for ${key}:`, error);
                    }
                }
            }
        }
        
        return savedStates;
    }, []);

    const clearWalletState = useCallback((chainId?: string) => {
        if (typeof window !== "undefined") {
            if (chainId) {
                localStorage.removeItem(`spectra_ibc_wallet_${chainId}`);
            } else {
                const keys = [];
                for (let i = 0; i < localStorage.length; i++) {
                    const key = localStorage.key(i);
                    if (key?.startsWith("spectra_ibc_wallet_")) {
                        keys.push(key);
                    }
                }
                for (const key of keys) {
                    localStorage.removeItem(key);
                }
            }
        }
    }, []);

    const autoReconnectToChain = useCallback(
        async (savedState: WalletConnectionState & { chainConfig: ClientChain }) => {
            try {
                const wallet = await getWalletFromProvider(savedState.walletType);
                const key = await wallet.getKey(savedState.chainId);

                setConnections((prev) => {
                    const next = new Map(prev);
                    next.set(savedState.chainId, {
                        chainId: savedState.chainId,
                        address: key.bech32Address,
                        chainConfig: savedState.chainConfig,
                    });
                    return next;
                });
                
                if (!walletType) {
                    setWalletType(savedState.walletType);
                }
            } catch (error) {
                console.log("Auto-reconnect failed for chain:", savedState.chainId, error);
                clearWalletState(savedState.chainId);
            }
        },
        [clearWalletState, walletType],
    );

    // Initialize on component mount - restore all saved connections
    useEffect(() => {
        const autoReconnect = async () => {
            try {
                const savedStates = loadAllWalletStates();
                if (savedStates.length === 0) {
                    return;
                }

                await Promise.allSettled(
                    savedStates.map((savedState) => autoReconnectToChain(savedState))
                );
            } catch (error) {
                console.log("Auto-reconnect failed:", error);
            }
        };

        autoReconnect();
    }, [autoReconnectToChain, loadAllWalletStates]);

    const suggestChain = async (
        chainConfigs: ClientChain[],
        walletType: WalletType = WalletType.KEPLR,
    ) => {
        try {
            const wallet = await getWalletFromProvider(walletType);

            for (const chainConfig of chainConfigs) {
                const keplr = chainConfig.keplr_chain_config;
                
                const keplrChainInfo = {
                    chainId: keplr.chain_id,
                    chainName: keplr.chain_name,
                    chainSymbolImageUrl: keplr.chain_symbol_image_url,
                    rpc: keplr.rpc,
                    rest: keplr.rest,
                    bip44: {
                        coinType: keplr.bip44.coin_type,
                    },
                    bech32Config: {
                        bech32PrefixAccAddr: keplr.bech32_config.bech32_prefix_acc_addr,
                        bech32PrefixAccPub: keplr.bech32_config.bech32_prefix_acc_pub,
                        bech32PrefixValAddr: keplr.bech32_config.bech32_prefix_val_addr,
                        bech32PrefixValPub: keplr.bech32_config.bech32_prefix_val_pub,
                        bech32PrefixConsAddr: keplr.bech32_config.bech32_prefix_cons_addr,
                        bech32PrefixConsPub: keplr.bech32_config.bech32_prefix_cons_pub,
                    },
                    currencies: keplr.currencies.map((cur) => ({
                        coinDenom: cur.coin_denom,
                        coinMinimalDenom: cur.coin_minimal_denom,
                        coinDecimals: cur.coin_decimals,
                    })),
                    feeCurrencies: keplr.fee_currencies.map((cur) => ({
                        coinDenom: cur.coin_denom,
                        coinMinimalDenom: cur.coin_minimal_denom,
                        coinDecimals: cur.coin_decimals,
                        gasPriceStep: {
                            low: cur.gas_price_step.low,
                            average: cur.gas_price_step.average,
                            high: cur.gas_price_step.high,
                        },
                    })),
                    stakeCurrency: keplr.currencies[0] ? {
                        coinDenom: keplr.currencies[0].coin_denom,
                        coinMinimalDenom: keplr.currencies[0].coin_minimal_denom,
                        coinDecimals: keplr.currencies[0].coin_decimals,
                    } : undefined,
                };

                await wallet.experimentalSuggestChain(keplrChainInfo);
            }
        } catch (error) {
            console.error("Failed to suggest chain:", error);
            throw error;
        }
    };

    const connect = async (chainConfigs: ClientChain[], walletTypeParam: WalletType = WalletType.KEPLR) => {
        try {
            const existingWalletType = localStorage.getItem("spectra_ibc_wallet_type");
            if (existingWalletType && existingWalletType !== walletTypeParam) {
                throw new Error(
                    `Already connected with ${existingWalletType}. Please disconnect first before connecting with ${walletTypeParam}.`,
                );
            }

            const wallet = await getWalletFromProvider(walletTypeParam);
            const newConnections = new Map(connections);
            
            for (const chainConfig of chainConfigs) {
                try {
                    const chainId = chainConfig.id;
                    
                    try {
                        await wallet.enable(chainId);
                    } catch {
                        console.log("Chain not found, suggesting chain...");
                        await suggestChain([chainConfig], walletTypeParam);
                        await wallet.enable(chainId);
                    }

                    const key = await wallet.getKey(chainId);

                    newConnections.set(chainId, {
                        chainId: chainId,
                        address: key.bech32Address,
                        chainConfig: chainConfig,
                    });

                    saveWalletState({
                        walletType: walletTypeParam,
                        address: key.bech32Address,
                        chainId: chainId,
                        chainConfig: chainConfig,
                    });
                } catch (error) {
                    console.error(`Failed to connect to chain ${chainConfig.id}:`, error);
                }
            }

            setConnections(newConnections);
            setWalletType(walletTypeParam);
        } catch (error) {
            console.error("Failed to connect wallet:", error);
            throw error;
        }
    };

    const disconnect = (chainIds?: string[]) => {
        if (chainIds && chainIds.length > 0) {
            // Disconnect specific chains
            setConnections((prev) => {
                const next = new Map(prev);
                for (const chainId of chainIds) {
                    next.delete(chainId);
                    clearWalletState(chainId);
                }
                return next;
            });

            // If no connections left, clear wallet type
            if (connections.size - chainIds.length === 0) {
                setWalletType(null);
            }
        } else {
            // Disconnect all
            setConnections(new Map());
            setWalletType(null);
            clearWalletState();
        }
    };

    /**
     * Generic transaction method - handles ALL transaction types
     * This replaces ibcTransfer, poolmanagerSwapTokenIn, etc.
     */
    const sendTransaction = async (
        chainId: string,
        messages: EncodeObject[],
        options: TransactionOptions = {}
    ): Promise<TransactionResult> => {
        const { fee = "auto", memo = "", gasAdjustment = 1.5, chainConfig: chainConfigParam } = options;

        // Get connection for this chain
        const connection = connections.get(chainId);
        const configToUse = chainConfigParam || connection?.chainConfig;

        if (!configToUse || !connection || !walletType) {
            throw new Error(`Wallet not connected to chain ${chainId}`);
        }

        const address = connection.address;

        try {
            const wallet = await getWalletFromProvider(walletType);
            const key = await wallet.getKey(chainId);
            const isLedger = key.isNanoLedger;
            const offlineSigner = await wallet.getOfflineSignerAuto(chainId);

            // Create registry with Osmosis types
            const registry = new Registry(defaultRegistryTypes);
            registry.register(
                "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn",
                MsgSwapExactAmountIn as GeneratedType,
            );
            registry.register(
                "/osmosis.poolmanager.v1beta1.MsgSplitRouteSwapExactAmountIn",
                MsgSplitRouteSwapExactAmountIn as GeneratedType,
            );

            const aminoTypes = new AminoTypes(createDefaultAminoConverters());

            // Get fee currency and gas price
            const feeCurrency = configToUse.keplr_chain_config.fee_currencies[0];
            if (!feeCurrency) {
                throw new Error("No fee currency found in chain config");
            }

            const gasPrice = GasPrice.fromString(
                `${feeCurrency.gas_price_step.average}${feeCurrency.coin_minimal_denom}`,
            );

            const randomHealthyRpc = getRandomHealthyRpc(chainId) as string;
            const client = await SigningStargateClient.connectWithSigner(
                randomHealthyRpc,
                offlineSigner,
                { gasPrice, registry, aminoTypes },
            );

            let finalFee: StdFee;
            if (fee === "auto") {
                const gasEstimated = await client.simulate(address, messages, memo);
                finalFee = calculateFee(Math.round(gasEstimated * gasAdjustment), gasPrice);
            } else {
                finalFee = fee;
            }

            if (isLedger) {
                console.log("Ledger account detected, providing explicit SignerData.");
                const clientForQuery = await StargateClient.connect(randomHealthyRpc);
                const account = await clientForQuery.getAccount(address);
                if (!account) {
                    throw new Error("Could not retrieve account details for signing.");
                }

                const signed = await client.sign(address, messages, finalFee, memo, {
                    accountNumber: account.accountNumber,
                    sequence: account.sequence,
                    chainId,
                });
                const result = await client.broadcastTx(TxRaw.encode(signed).finish());

                return {
                    transactionHash: result.transactionHash,
                    code: result.code,
                    height: result.height,
                };
            }

            const result = await client.signAndBroadcast(address, messages, finalFee, memo);

            return {
                transactionHash: result.transactionHash,
                code: result.code,
                height: result.height,
            };
        } catch (error) {
            console.error("Failed to sign and broadcast transaction:", error);
            throw error;
        }
    };

    return (
        <WalletContext.Provider
            value={{
                isConnected,
                chains,
                walletType,
                getAddress,
                getChainConfig,
                isConnectedToChain,
                connection: {
                    connect,
                    disconnect,
                    suggestChain,
                },
                sendTransaction,
            }}
        >
            {children}
        </WalletContext.Provider>
    );
};


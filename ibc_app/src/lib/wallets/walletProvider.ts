/**
 * Unified wallet provider types and interfaces for Cosmos wallets
 * Supports Keplr, Leap, and Cosmostation wallets
 */

import type { Keplr } from "@keplr-wallet/types";

/**
 * Supported wallet types
 */
export enum WalletType {
    KEPLR = "keplr",
    LEAP = "leap",
    COSMOSTATION = "cosmostation",
}

/**
 * Wallet metadata for display purposes
 */
export interface WalletInfo {
    type: WalletType;
    name: string;
    logo: string;
    downloadUrl: string;
}

/**
 * Available wallet configurations
 */
export const WALLET_INFO: Record<WalletType, WalletInfo> = {
    [WalletType.KEPLR]: {
        type: WalletType.KEPLR,
        name: "Keplr Wallet",
        logo: "/Keplr_logo.png",
        downloadUrl: "https://www.keplr.app/download",
    },
    [WalletType.LEAP]: {
        type: WalletType.LEAP,
        name: "Leap Wallet",
        logo: "/Leap_logo.png",
        downloadUrl: "https://www.leapwallet.io/download",
    },
    [WalletType.COSMOSTATION]: {
        type: WalletType.COSMOSTATION,
        name: "Cosmostation Wallet",
        logo: "/Cosmostation_logo.png",
        downloadUrl: "https://www.cosmostation.io/wallet",
    },
};

/**
 * Unified wallet provider interface
 * All Cosmos wallets (Keplr, Leap, Cosmostation) implement similar interfaces
 */
export type UnifiedWalletProvider = Keplr;

/**
 * Window extensions for wallet providers
 */
declare global {
    interface Window {
        keplr?: UnifiedWalletProvider;
        leap?: UnifiedWalletProvider;
        cosmostation?: {
            providers?: {
                keplr?: UnifiedWalletProvider;
            };
        };
    }
}

/**
 * Wallet connection state stored in localStorage
 */
export interface WalletConnectionState {
    walletType: WalletType;
    address: string;
    chainId: string;
    chainConfig: unknown; // Will be typed as ChainConfig in the context
}

/**
 * Get the wallet provider from the window object based on wallet type
 * @param walletType The type of wallet to get
 * @returns The wallet provider or null if not found
 */
export const getWalletProvider = (walletType: WalletType): UnifiedWalletProvider | null => {
    if (typeof window === "undefined") {
        return null;
    }

    switch (walletType) {
        case WalletType.KEPLR:
            return window.keplr || null;

        case WalletType.LEAP:
            return window.leap || null;

        case WalletType.COSMOSTATION:
            // Cosmostation exposes a Keplr-compatible API at window.cosmostation.providers.keplr
            return window.cosmostation?.providers?.keplr || null;

        default:
            return null;
    }
};

/**
 * Check if a specific wallet is installed
 * @param walletType The type of wallet to check
 * @returns true if the wallet is installed, false otherwise
 */
export const isWalletInstalled = (walletType: WalletType): boolean => {
    return getWalletProvider(walletType) !== null;
};

/**
 * Get all installed wallets
 * @returns Array of installed wallet types
 */
export const getInstalledWallets = (): WalletType[] => {
    const wallets: WalletType[] = [];

    for (const walletType of Object.values(WalletType)) {
        if (isWalletInstalled(walletType)) {
            wallets.push(walletType);
        }
    }

    return wallets;
};

/**
 * Wait for a wallet to be injected into the window object
 * Useful for handling race conditions where the wallet extension loads after the page
 * @param walletType The type of wallet to wait for
 * @param timeoutMs Maximum time to wait in milliseconds
 * @returns Promise that resolves to the wallet provider or null if timeout occurs
 */
export const waitForWallet = async (
    walletType: WalletType,
    timeoutMs: number = 3000,
): Promise<UnifiedWalletProvider | null> => {
    const startTime = Date.now();

    while (Date.now() - startTime < timeoutMs) {
        const provider = getWalletProvider(walletType);
        if (provider) {
            return provider;
        }
        // Wait 100ms before checking again
        await new Promise((resolve) => setTimeout(resolve, 100));
    }

    return null;
};

/**
 * Get a wallet provider with a fallback wait period
 * First tries to get the wallet immediately, then waits if not found
 * @param walletType The type of wallet to get
 * @returns Promise that resolves to the wallet provider
 * @throws Error if wallet is not found after waiting
 */
export const getWalletProviderAsync = async (
    walletType: WalletType,
): Promise<UnifiedWalletProvider> => {
    // First try to get immediately
    let provider = getWalletProvider(walletType);
    if (provider) {
        return provider;
    }

    // If not found, wait a bit (wallet might still be loading)
    provider = await waitForWallet(walletType, 1000);
    if (provider) {
        return provider;
    }

    // Still not found, throw error
    throw new Error(
        `${walletType} wallet is not installed. Please install the ${walletType} extension.`,
    );
};

/**
 * Validate that a wallet type is supported
 * @param walletType String to validate
 * @returns true if valid wallet type
 */
export const isValidWalletType = (walletType: string): walletType is WalletType => {
    return Object.values(WalletType).includes(walletType as WalletType);
};

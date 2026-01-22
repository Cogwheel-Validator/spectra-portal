import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

// ============================================================================
// Particle Utilities
// ============================================================================

export function cn(...inputs: ClassValue[]) {
    return twMerge(clsx(inputs));
}

// ============================================================================
// Token Amount Conversion Utilities
// ============================================================================

/**
 * Converts a human-readable amount (e.g., "1.5") to base units (smallest denomination).
 * Handles tokens with varying decimal places (e.g., 6 for USDC, 18 for EVM tokens).
 *
 * @param humanAmount - The human-readable amount as a string (e.g., "1.5", "0.001")
 * @param decimals - The number of decimal places for the token (default: 6)
 * @returns The amount in base units as a string (e.g., "1500000" for 1.5 with 6 decimals)
 */
export function humanToBaseUnits(humanAmount: string, decimals: number = 6): string {
    if (!humanAmount || humanAmount === "0" || humanAmount === "") {
        return "0";
    }

    try {
        const parts = humanAmount.split(".");
        const integerPart = parts[0] || "0";
        const decimalPart = (parts[1] || "").padEnd(decimals, "0").slice(0, decimals);

        // Combine to get the full amount in base units
        const fullAmount = integerPart + decimalPart;
        // Remove leading zeros but keep at least one digit
        return BigInt(fullAmount).toString();
    } catch {
        return "0";
    }
}

/**
 * Converts base units (smallest denomination) to a human-readable amount.
 * Handles tokens with varying decimal places.
 *
 * @param baseUnits - The amount in base units as a string (e.g., "1500000")
 * @param decimals - The number of decimal places for the token (default: 6)
 * @returns The human-readable amount as a number
 */
export function baseUnitsToHuman(baseUnits: string, decimals: number = 6): number {
    if (!baseUnits || baseUnits === "0" || baseUnits === "") {
        return 0;
    }

    try {
        return Number(baseUnits) / 10 ** decimals;
    } catch {
        return 0;
    }
}

/**
 * Formats a base unit amount for display with proper decimal places.
 *
 * @param baseUnits - The amount in base units as a string
 * @param decimals - The number of decimal places for the token (default: 6)
 * @param maxDisplayDecimals - Maximum decimal places to show (default: 4)
 * @returns Formatted string for display
 */
export function formatBaseUnitsForDisplay(
    baseUnits: string,
    decimals: number = 6,
    maxDisplayDecimals: number = 4,
): string {
    const human = baseUnitsToHuman(baseUnits, decimals);
    return human.toFixed(Math.min(decimals, maxDisplayDecimals));
}

// ============================================================================
// URL Utilities
// ============================================================================

/**
 * Builds an explorer URL for a transaction, replacing placeholders in the path.
 *
 * @param baseUrl - The base URL of the explorer (e.g., "https://mintscan.io")
 * @param transactionPath - The path template with placeholder (e.g., "osmosis/txs/{tx_hash}")
 * @param txHash - The transaction hash to insert
 * @returns The complete explorer URL
 */
export function buildExplorerTxUrl(
    baseUrl: string,
    transactionPath: string,
    txHash: string,
): string {
    // Replace the {tx_hash} placeholder with the actual hash
    const pathWithHash = transactionPath.replace("{tx_hash}", txHash);

    // Ensure proper URL construction (no double slashes, proper joining)
    const cleanBase = baseUrl.endsWith("/") ? baseUrl.slice(0, -1) : baseUrl;
    const cleanPath = pathWithHash.startsWith("/") ? pathWithHash.slice(1) : pathWithHash;

    return `${cleanBase}/${cleanPath}`;
}

/**
 * Builds an explorer URL for an account address, replacing placeholders in the path.
 *
 * @param baseUrl - The base URL of the explorer
 * @param accountPath - The path template with placeholder (e.g., "osmosis/address/{address}")
 * @param address - The account address to insert
 * @returns The complete explorer URL
 */
export function buildExplorerAccountUrl(
    baseUrl: string,
    accountPath: string,
    address: string,
): string {
    const pathWithAddress = accountPath.replace("{address}", address);

    const cleanBase = baseUrl.endsWith("/") ? baseUrl.slice(0, -1) : baseUrl;
    const cleanPath = pathWithAddress.startsWith("/") ? pathWithAddress.slice(1) : pathWithAddress;

    return `${cleanBase}/${cleanPath}`;
}

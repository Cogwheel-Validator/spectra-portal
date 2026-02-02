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

// ============================================================================
// Error Detection Utilities
// ============================================================================

/**
 * Result of slippage error analysis
 */
export interface SlippageErrorInfo {
    /** Whether the error is a slippage-related failure */
    isSlippageError: boolean;
    /** The calculated amount that would have been received */
    calculatedAmount?: string;
    /** The minimum amount that was required */
    minAmount?: string;
    /** Shortfall as a percentage (negative means calculated was less than min) */
    shortfallPercent?: number;
    /** Suggested slippage to use (in basis points) to avoid this error */
    suggestedSlippageBps?: number;
}

/**
 * Detects if an error message is related to slippage/price movement.
 * Parses the error to extract useful information for the user.
 *
 * Common error patterns:
 * - "token amount calculated (X) is lesser than min amount (Y)"
 * - "minimum receive amount: X, actual: Y"
 * - "slippage exceeded"
 *
 * Usage:
 * - For now it should mark the Osmosis swap error I think?
 * - Should also cover some edge cases
 * - Todo: change this section a bit
 *
 * @param errorMessage - The error message from the transaction
 * @returns SlippageErrorInfo with analysis results
 */
export function parseSlippageError(errorMessage: string): SlippageErrorInfo {
    // Pattern 1: Osmosis swap error
    // "token amount calculated (361937) is lesser than min amount (363079)"
    const osmosisPattern = /token amount calculated \((\d+)\) is lesser than min amount \((\d+)\)/i;
    const osmosisMatch = errorMessage.match(osmosisPattern);

    if (osmosisMatch) {
        const calculatedAmount = osmosisMatch[1];
        const minAmount = osmosisMatch[2];
        const calculated = BigInt(calculatedAmount);
        const min = BigInt(minAmount);

        // Calculate shortfall percentage
        // (calculated - min) / min * 100 (negative means shortfall)
        const shortfallPercent = Number(((calculated - min) * BigInt(10000)) / min) / 100;

        // Suggest slippage: current shortfall + 0.5% buffer
        // E.g., if calculated was 0.31% less than min, suggest at least 0.81% (81 bps)
        const suggestedSlippageBps = Math.ceil(Math.abs(shortfallPercent) * 100) + 50;

        return {
            isSlippageError: true,
            calculatedAmount,
            minAmount,
            shortfallPercent,
            suggestedSlippageBps: Math.max(suggestedSlippageBps, 100), // Minimum 1%
        };
    }

    // Pattern 2: Generic slippage exceeded
    const slippagePattern = /slippage (exceeded|tolerance)/i;
    if (slippagePattern.test(errorMessage)) {
        return {
            isSlippageError: true,
            // Suggest 2% if we can't parse the exact amounts
            // This kinda sucks at the momen becaus the user could set higher slipage but it is okay
            // This is just some idea to try to standardize this in the future so that teh Portal app doesn't just
            // rely on the Osmosis SQS for example
            // TODO leave this part as it is, it probably won't be used in this form but to future me this will need
            // a rework when some new pathfinder brokers are added
            suggestedSlippageBps: 200,
        };
    }

    // Pattern 3: Minimum receive amount error
    const minReceivePattern = /minimum receive amount[:\s]+(\d+)[,\s]+actual[:\s]+(\d+)/i;
    const minReceiveMatch = errorMessage.match(minReceivePattern);

    if (minReceiveMatch) {
        const minAmount = minReceiveMatch[1];
        const calculatedAmount = minReceiveMatch[2];
        const calculated = BigInt(calculatedAmount);
        const min = BigInt(minAmount);

        const shortfallPercent = Number(((calculated - min) * BigInt(10000)) / min) / 100;
        const suggestedSlippageBps = Math.ceil(Math.abs(shortfallPercent) * 100) + 50;

        return {
            isSlippageError: true,
            calculatedAmount,
            minAmount,
            shortfallPercent,
            suggestedSlippageBps: Math.max(suggestedSlippageBps, 100),
        };
    }

    return { isSlippageError: false };
}

/**
 * Formats a slippage error into a user-friendly message with suggestions.
 *
 * @param errorInfo - The parsed slippage error info
 * @param tokenSymbol - Optional token symbol for display
 * @param decimals - Token decimals for amount formatting
 * @returns User-friendly error message with suggestions
 */
export function formatSlippageErrorMessage(
    errorInfo: SlippageErrorInfo,
    tokenSymbol?: string,
    decimals: number = 6,
): string {
    if (!errorInfo.isSlippageError) {
        return "Transaction failed";
    }

    const parts: string[] = [];

    parts.push("Transaction failed due to price movement (slippage).");

    if (errorInfo.calculatedAmount && errorInfo.minAmount && errorInfo.shortfallPercent) {
        const calculated = baseUnitsToHuman(errorInfo.calculatedAmount, decimals);
        const min = baseUnitsToHuman(errorInfo.minAmount, decimals);
        const symbol = tokenSymbol ? ` ${tokenSymbol}` : "";

        parts.push(
            `Expected at least ${min.toLocaleString()}${symbol} but would only receive ${calculated.toLocaleString()}${symbol} (${Math.abs(errorInfo.shortfallPercent).toFixed(2)}% less).`,
        );
    }

    if (errorInfo.suggestedSlippageBps) {
        const suggestedPercent = errorInfo.suggestedSlippageBps / 100;
        parts.push(
            `Consider increasing slippage tolerance to ${suggestedPercent.toFixed(1)}% or higher.`,
        );
    }

    return parts.join(" ");
}

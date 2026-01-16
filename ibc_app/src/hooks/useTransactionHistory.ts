"use client";

import { useCallback, useEffect, useState } from "react";
import {
    ConnectToDb,
    LoadAllTransactions,
    type TransactionRecord,
    type TransactionUpdate,
    UpdateTransactionStatus,
} from "@/lib/indexDb/dbManager";

/**
 * Maximum age for resumable transactions (4 hours in milliseconds)
 */
const MAX_RESUMABLE_AGE_MS = 4 * 60 * 60 * 1000;

/**
 * Transaction with computed properties for UI
 */
export interface TransactionWithMeta extends TransactionRecord {
    isResumable: boolean;
    isRetryable: boolean;
    ageMs: number;
    formattedAge: string;
}

/**
 * State returned by useTransactionHistory hook
 */
export interface TransactionHistoryState {
    transactions: TransactionWithMeta[];
    isLoading: boolean;
    error: string | null;
    refresh: () => Promise<void>;
    updateStatus: (update: TransactionUpdate) => Promise<void>;
    resumableCount: number;
}

/**
 * Formats the age of a transaction into a human-readable string
 */
function formatAge(ageMs: number): string {
    const seconds = Math.floor(ageMs / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    const days = Math.floor(hours / 24);

    if (days > 0) {
        return `${days}d ago`;
    }
    if (hours > 0) {
        return `${hours}h ago`;
    }
    if (minutes > 0) {
        return `${minutes}m ago`;
    }
    return "Just now";
}

/**
 * Checks if an error message indicates a retryable error
 */
function isRetryableError(error: string | null): boolean {
    if (!error) return false;
    const retryablePatterns = [
        /out of gas/i,
        /insufficient/i,
        /timeout/i,
        /network/i,
        /connection/i,
        /rate limit/i,
    ];
    return retryablePatterns.some((pattern) => pattern.test(error));
}

/**
 * Hook to manage transaction history from IndexedDB
 * @param limit - Maximum number of transactions to fetch
 * @returns Transaction history state with refresh and update capabilities
 */
export function useTransactionHistory(limit: number = 50): TransactionHistoryState {
    const [transactions, setTransactions] = useState<TransactionWithMeta[]>([]);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [db, setDb] = useState<IDBDatabase | null>(null);

    // Initialize database connection
    useEffect(() => {
        let mounted = true;

        const initDb = async () => {
            try {
                const database = await ConnectToDb();
                if (mounted) {
                    setDb(database);
                }
            } catch (err) {
                if (mounted) {
                    setError(err instanceof Error ? err.message : "Failed to connect to database");
                    setIsLoading(false);
                }
            }
        };

        initDb();

        return () => {
            mounted = false;
        };
    }, []);

    // Fetch transactions when database is ready
    const refresh = useCallback(async () => {
        if (!db) return;

        setIsLoading(true);
        setError(null);

        try {
            const rawTransactions = await LoadAllTransactions(db);
            const now = Date.now();

            const withMeta: TransactionWithMeta[] = rawTransactions.slice(0, limit).map((tx) => {
                const timestamp = new Date(tx.timestamp).getTime();
                const ageMs = now - timestamp;
                const isWithinResumeWindow = ageMs < MAX_RESUMABLE_AGE_MS;

                return {
                    ...tx,
                    ageMs,
                    formattedAge: formatAge(ageMs),
                    isResumable:
                        tx.status === "in-progress" &&
                        isWithinResumeWindow &&
                        tx.currentStep < tx.totalSteps,
                    isRetryable: tx.status === "failed" && isRetryableError(tx.error),
                };
            });

            setTransactions(withMeta);
        } catch (err) {
            setError(err instanceof Error ? err.message : "Failed to load transactions");
        } finally {
            setIsLoading(false);
        }
    }, [db, limit]);

    // Load transactions when db is ready
    useEffect(() => {
        if (db) {
            refresh();
        }
    }, [db, refresh]);

    // Update transaction status
    const updateStatus = useCallback(
        async (update: TransactionUpdate) => {
            if (!db) {
                throw new Error("Database not connected");
            }

            await UpdateTransactionStatus(db, update);
            await refresh();
        },
        [db, refresh],
    );

    // Count resumable transactions
    const resumableCount = transactions.filter((tx) => tx.isResumable).length;

    return {
        transactions,
        isLoading,
        error,
        refresh,
        updateStatus,
        resumableCount,
    };
}

/**
 * Hook to get only resumable (in-progress, < 4 hours old) transactions
 */
export function useResumableTransactions(): {
    transactions: TransactionWithMeta[];
    isLoading: boolean;
} {
    const { transactions, isLoading } = useTransactionHistory();

    const resumable = transactions.filter((tx) => tx.isResumable);

    return {
        transactions: resumable,
        isLoading,
    };
}

"use client";

import { type Client, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
    FindPathRequest,
    FindPathResponse,
} from "@/lib/generated/pathfinder/pathfinder_route_pb";
import { PathfinderService } from "@/lib/generated/pathfinder/pathfinder_route_pb";
import { useDebounce } from "./useDebounce";

/**
 * Parameters for the pathfinder query
 */
export interface PathfinderQueryParams {
    chainFrom: string;
    tokenFromDenom: string;
    amountIn: string;
    chainTo: string;
    tokenToDenom?: string;
    senderAddress: string;
    receiverAddress: string;
    smartRoute?: boolean;
    slippageBps?: number;
}

/**
 * Configuration options for the pathfinder query hook
 */
export interface PathfinderQueryOptions {
    /** Debounce delay in milliseconds for amount changes (default: 1000ms) */
    debounceMs?: number;
    /** Auto-refresh interval in milliseconds (default: 30000ms = 30s, 0 to disable) */
    autoRefreshMs?: number;
    /** Time after which a quote is considered stale in milliseconds (default: 15000ms = 15s) */
    staleAfterMs?: number;
}

/**
 * State returned by the usePathfinderQuery hook
 */
export interface PathfinderQueryState {
    data: FindPathResponse | null;
    isLoading: boolean;
    isPending: boolean;
    error: string | null;
    isStale: boolean;
    lastFetchedAt: number | null;
    quoteAgeSeconds: number | null;
    refetch: () => Promise<void>;
    refetchFresh: () => Promise<FindPathResponse | null>;
}

/**
 * Creates a pathfinder client instance
 */
function createPathfinderClient(): Client<typeof PathfinderService> {
    const baseUrl = process.env.NEXT_PUBLIC_PATHFINDER_RPC_URL || "http://localhost:8080";
    const transport = createConnectTransport({ baseUrl });
    return createClient(PathfinderService, transport);
}

const DEFAULT_OPTIONS: Required<PathfinderQueryOptions> = {
    debounceMs: 1000,
    autoRefreshMs: 30000,
    staleAfterMs: 15000,
};

/**
 * Hook to query the pathfinder for route information
 * Automatically debounces the amount input to prevent excessive queries
 * Supports auto-refresh and stale quote detection for better UX
 *
 * @param params - The query parameters
 * @param enabled - Whether the query should be enabled
 * @param options - Configuration options (debounce, auto-refresh, stale threshold)
 * @returns The query state with data, loading, error, staleness info, and refetch functions
 */
export function usePathfinderQuery(
    params: PathfinderQueryParams | null,
    enabled: boolean = true,
    options: PathfinderQueryOptions | number = DEFAULT_OPTIONS,
): PathfinderQueryState {
    // Handle backwards compatibility: if number is passed, treat as debounceMs
    const opts: Required<PathfinderQueryOptions> =
        typeof options === "number"
            ? { ...DEFAULT_OPTIONS, debounceMs: options }
            : { ...DEFAULT_OPTIONS, ...options };

    const [data, setData] = useState<FindPathResponse | null>(null);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [lastFetchedAt, setLastFetchedAt] = useState<number | null>(null);
    const [now, setNow] = useState(() => Date.now());

    // Debounce the amount to prevent excessive API calls
    const debouncedAmount = useDebounce(params?.amountIn ?? "", opts.debounceMs);
    const isPending = params?.amountIn !== debouncedAmount;

    // Keep track of the latest request to handle race conditions
    const requestIdRef = useRef(0);

    // Memoize the client to avoid recreating on each render
    const client = useMemo(() => createPathfinderClient(), []);

    // Calculate staleness
    const quoteAgeMs = lastFetchedAt ? now - lastFetchedAt : null;
    const quoteAgeSeconds = quoteAgeMs !== null ? Math.floor(quoteAgeMs / 1000) : null;
    const isStale = quoteAgeMs !== null && quoteAgeMs > opts.staleAfterMs;

    useEffect(() => {
        const interval = setInterval(() => setNow(Date.now()), 1000);
        return () => clearInterval(interval);
    }, []);

    const fetchRouteInternal = useCallback(
        async (amountOverride?: string): Promise<FindPathResponse | null> => {
            const amount = amountOverride ?? debouncedAmount;

            if (!params || !enabled) {
                setData(null);
                return null;
            }

            // Validate required fields
            if (
                !params.chainFrom ||
                !params.chainTo ||
                !params.tokenFromDenom ||
                !params.senderAddress ||
                !params.receiverAddress
            ) {
                setData(null);
                return null;
            }

            // Validate amount
            const amountNum = Number.parseFloat(amount);
            if (Number.isNaN(amountNum) || amountNum <= 0) {
                setData(null);
                return null;
            }

            // Increment request ID to handle race conditions
            const currentRequestId = ++requestIdRef.current;

            setIsLoading(true);
            setError(null);

            try {
                const request: Partial<FindPathRequest> = {
                    chainFrom: params.chainFrom,
                    tokenFromDenom: params.tokenFromDenom,
                    amountIn: amount,
                    chainTo: params.chainTo,
                    tokenToDenom: params.tokenToDenom || "",
                    senderAddress: params.senderAddress,
                    receiverAddress: params.receiverAddress,
                    smartRoute: params.smartRoute ?? false,
                    slippageBps: params.slippageBps ?? 100, // Default 1% slippage
                };

                const response = await client.findPath(request as FindPathRequest);

                // Only update state if this is still the latest request
                if (currentRequestId === requestIdRef.current) {
                    if (response.success) {
                        setData(response);
                        setError(null);
                        setLastFetchedAt(Date.now());
                        return response;
                    }
                    setData(null);
                    setError(response.errorMessage || "Failed to find route");
                    return null;
                }
                return null;
            } catch (err) {
                // Only update state if this is still the latest request
                if (currentRequestId === requestIdRef.current) {
                    const errorMessage =
                        err instanceof Error ? err.message : "Unknown error occurred";
                    setError(errorMessage);
                    setData(null);
                }
                return null;
            } finally {
                // Only update loading state if this is still the latest request
                if (currentRequestId === requestIdRef.current) {
                    setIsLoading(false);
                }
            }
        },
        [client, params, enabled, debouncedAmount],
    );

    // Debounced fetch (for UI typing)
    const fetchRoute = useCallback(async () => {
        await fetchRouteInternal();
    }, [fetchRouteInternal]);

    // Fresh fetch - bypasses debounce, uses current amount directly
    // Use this right before executing a transaction
    const refetchFresh = useCallback(async (): Promise<FindPathResponse | null> => {
        if (!params?.amountIn) return null;
        return fetchRouteInternal(params.amountIn);
    }, [fetchRouteInternal, params?.amountIn]);

    // Fetch route when debounced amount changes
    useEffect(() => {
        fetchRoute();
    }, [fetchRoute]);

    // Auto-refresh interval (only when enabled and we have valid data)
    useEffect(() => {
        if (!enabled || !data || opts.autoRefreshMs === 0) return;

        const interval = setInterval(() => {
            // Only auto-refresh if we have params and data
            if (params && data) {
                fetchRouteInternal();
            }
        }, opts.autoRefreshMs);

        return () => clearInterval(interval);
    }, [enabled, data, params, opts.autoRefreshMs, fetchRouteInternal]);

    return {
        data,
        isLoading,
        isPending,
        error,
        isStale,
        lastFetchedAt,
        quoteAgeSeconds,
        refetch: fetchRoute,
        refetchFresh,
    };
}

/**
 * Extracts route type from FindPathResponse
 */
export function getRouteType(
    response: FindPathResponse,
): "direct" | "indirect" | "broker_swap" | null {
    if (!response.success) return null;
    const routeCase = response.route.case;
    if (routeCase === "brokerSwap") return "broker_swap";
    return routeCase ?? null;
}

/**
 * Extracts the path (chain IDs in order) from FindPathResponse
 */
export function getRoutePath(response: FindPathResponse): string[] {
    if (!response.success) return [];

    switch (response.route.case) {
        case "direct": {
            const transfer = response.route.value.transfer;
            if (transfer) {
                return [transfer.fromChain, transfer.toChain];
            }
            return [];
        }
        case "indirect":
            return response.route.value.path;
        case "brokerSwap":
            return response.route.value.path;
        default:
            return [];
    }
}

/**
 * Checks if the route supports PFM (Packet Forwarding Middleware)
 */
export function routeSupportsPfm(response: FindPathResponse): boolean {
    if (!response.success) return false;

    switch (response.route.case) {
        case "direct":
            return false; // Direct routes don't need PFM
        case "indirect":
            return response.route.value.supportsPfm;
        case "brokerSwap":
            return response.route.value.outboundSupportsPfm;
        default:
            return false;
    }
}

/**
 * Gets the total number of steps/transactions required for the route
 */
export function getRouteStepCount(response: FindPathResponse, mode: "manual" | "smart"): number {
    if (!response.success) return 0;

    switch (response.route.case) {
        case "direct":
            return 1;
        case "indirect": {
            const route = response.route.value;
            if (mode === "smart" && route.supportsPfm) {
                return 1; // PFM allows single transaction
            }
            return route.legs.length;
        }
        case "brokerSwap": {
            const route = response.route.value;
            if (mode === "smart" && route.execution?.usesWasm) {
                // Smart contract WASM execution: 1 transaction for inbound+swap+outbound
                // TODO: this is a flawed logic, especially when there is a transaction
                // that can benefit partial use of smart transfers. For now leave it like this
                // until there is a proper way to address this from the pathfinder side.
                return 1;
            }
            // Manual mode: count each leg
            let steps = 0;
            if (route.inboundLegs.length > 0) steps += route.inboundLegs.length;
            if (route.swap) steps++; // Swap always counts as 1
            if (route.outboundLegs.length > 0) steps += route.outboundLegs.length;
            return steps;
        }
        default:
            return 0;
    }
}

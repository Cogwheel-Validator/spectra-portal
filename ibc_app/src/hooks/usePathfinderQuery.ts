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
    singleRoute?: boolean;
    slippageBps?: number;
}

/**
 * State returned by the usePathfinderQuery hook
 */
export interface PathfinderQueryState {
    data: FindPathResponse | null;
    isLoading: boolean;
    isPending: boolean;
    error: string | null;
    refetch: () => Promise<void>;
}

/**
 * Creates a pathfinder client instance
 */
function createPathfinderClient(): Client<typeof PathfinderService> {
    const baseUrl = process.env.NEXT_PUBLIC_PATHFINDER_RPC_URL || "http://localhost:8080";
    const transport = createConnectTransport({ baseUrl });
    return createClient(PathfinderService, transport);
}

/**
 * Hook to query the pathfinder for route information
 * Automatically debounces the amount input to prevent excessive queries
 *
 * @param params - The query parameters
 * @param enabled - Whether the query should be enabled
 * @param debounceMs - Debounce delay in milliseconds (default: 1000ms)
 * @returns The query state with data, loading, error, and refetch function
 */
export function usePathfinderQuery(
    params: PathfinderQueryParams | null,
    enabled: boolean = true,
    debounceMs: number = 1000,
): PathfinderQueryState {
    const [data, setData] = useState<FindPathResponse | null>(null);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    // Debounce the amount to prevent excessive API calls
    const debouncedAmount = useDebounce(params?.amountIn ?? "", debounceMs);
    const isPending = params?.amountIn !== debouncedAmount;

    // Keep track of the latest request to handle race conditions
    const requestIdRef = useRef(0);

    // Memoize the client to avoid recreating on each render
    const client = useMemo(() => createPathfinderClient(), []);

    const fetchRoute = useCallback(async () => {
        if (!params || !enabled) {
            setData(null);
            return;
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
            return;
        }

        // Validate amount
        const amountNum = Number.parseFloat(debouncedAmount);
        if (Number.isNaN(amountNum) || amountNum <= 0) {
            setData(null);
            return;
        }

        // Increment request ID to handle race conditions
        const currentRequestId = ++requestIdRef.current;

        setIsLoading(true);
        setError(null);

        try {
            const request: Partial<FindPathRequest> = {
                chainFrom: params.chainFrom,
                tokenFromDenom: params.tokenFromDenom,
                amountIn: debouncedAmount,
                chainTo: params.chainTo,
                tokenToDenom: params.tokenToDenom || "",
                senderAddress: params.senderAddress,
                receiverAddress: params.receiverAddress,
                singleRoute: params.singleRoute ?? true,
                slippageBps: params.slippageBps ?? 100, // Default 1% slippage
            };

            const response = await client.findPath(request as FindPathRequest);

            // Only update state if this is still the latest request
            if (currentRequestId === requestIdRef.current) {
                if (response.success) {
                    setData(response);
                    setError(null);
                } else {
                    setData(null);
                    setError(response.errorMessage || "Failed to find route");
                }
            }
        } catch (err) {
            // Only update state if this is still the latest request
            if (currentRequestId === requestIdRef.current) {
                const errorMessage = err instanceof Error ? err.message : "Unknown error occurred";
                setError(errorMessage);
                setData(null);
            }
        } finally {
            // Only update loading state if this is still the latest request
            if (currentRequestId === requestIdRef.current) {
                setIsLoading(false);
            }
        }
    }, [client, params, enabled, debouncedAmount]);

    // Fetch route when debounced amount changes
    useEffect(() => {
        fetchRoute();
    }, [fetchRoute]);

    return {
        data,
        isLoading,
        isPending,
        error,
        refetch: fetchRoute,
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
                return route.inboundLeg ? 1 : route.outboundLegs.length > 0 ? 1 : 1;
            }
            // Manual mode: count each leg
            let steps = 0;
            if (route.inboundLeg) steps++;
            steps++; // Swap always counts as 1
            steps += route.outboundLegs.length;
            return steps;
        }
        default:
            return 0;
    }
}

"use client";

import { createContext, type ReactNode, useContext } from "react";
import { useHealthyEndpoints } from "@/components/hooks/useHealthyApis";
import type { ClientChain } from "@/components/modules/tomlTypes";

interface HealthCheckContextValue {
    healthyApis: string[];
    healthyRpcs: string[];
    isLoading: boolean;
    isError: boolean;
    error: Error | null;
}

const HealthCheckContext = createContext<HealthCheckContextValue | undefined>(undefined);

/**
 * Hook to access health check data from context
 * Use this instead of useHealthyApis when inside a HealthCheckProvider
 */
export function useHealthCheck() {
    const context = useContext(HealthCheckContext);
    if (!context) {
        throw new Error("useHealthCheck must be used within a HealthCheckProvider");
    }
    return context;
}

/**
 * Optional: Try to get health check from context, return null if not available
 * Useful for components that can work with or without the provider
 */
export function useHealthCheckOptional() {
    return useContext(HealthCheckContext);
}

interface HealthCheckProviderProps {
    children: ReactNode;
    chainConfig: ClientChain;
}

/**
 * Centralized health check provider for a specific chain
 * Wrap this around chain-specific routes or components to share health check results
 */
export function HealthCheckProvider({ children, chainConfig }: HealthCheckProviderProps) {
    const restEndpoints = chainConfig.rest_endpoints.map((endpoint) => endpoint.url);
    const rpcEndpoints = chainConfig.rpc_endpoints.map((endpoint) => endpoint.url);
    const { data, isLoading, isError, error } = useHealthyEndpoints(
        restEndpoints,
        rpcEndpoints,
        chainConfig.id,
    );

    const value: HealthCheckContextValue = {
        healthyApis: data?.apis ?? [],
        healthyRpcs: data?.rpcs ?? [],
        isLoading,
        isError,
        error: error as Error | null,
    };

    return <HealthCheckContext.Provider value={value}>{children}</HealthCheckContext.Provider>;
}

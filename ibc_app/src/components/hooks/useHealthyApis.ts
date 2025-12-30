import { useQuery } from "@tanstack/react-query";
import logger from "@/lib/logger";

interface HealthCheckResponse {
    chainPath: string;
    apis: string[];
    rpcs: string[];
    cached: boolean;
    age: number;
}

/**
 * Centralized hook for checking and caching healthy APIs via server-side health checks
 * Results are shared across all components that use the same chain's APIs
 */
export function useHealthyApis(apis: string[], chainPath: string) {
    return useQuery<string[]>({
        queryKey: ["healthyApis", chainPath],
        queryFn: async () => {
            logger.info(`Fetching health status for ${apis.length} APIs`, { chainPath });

            const params = new URLSearchParams({
                chainPath,
                apis: apis.join(","),
            });

            const response = await fetch(`/api/health?${params.toString()}`);

            if (!response.ok) {
                throw new Error(`Health check failed: ${response.statusText}`);
            }

            const data: HealthCheckResponse = await response.json();

            logger.info(
                `Found ${data.apis.length} healthy APIs (cached: ${data.cached}, age: ${data.age}s)`,
                { chainPath, healthyApis: data.apis },
            );

            return data.apis;
        },
        enabled: !!apis && apis.length > 0,
        staleTime: 30 * 1000, // 30 seconds - server caches for 60s, so we can refetch more often
        gcTime: 10 * 60 * 1000, // 10 minutes - keep in cache
        refetchOnWindowFocus: false, // Don't recheck on every focus
        retry: 2,
    });
}

/**
 * Hook for checking both APIs and RPCs health via server-side health checks
 */
export function useHealthyEndpoints(apis: string[], rpcs: string[], chainPath: string) {
    return useQuery<{ apis: string[]; rpcs: string[] }>({
        queryKey: ["healthyEndpoints", chainPath],
        queryFn: async () => {
            logger.info(`Fetching health status for ${apis.length} APIs and ${rpcs.length} RPCs`, {
                chainPath,
            });

            const params = new URLSearchParams({
                chainPath,
            });

            if (apis.length > 0) {
                params.set("apis", apis.join(","));
            }

            if (rpcs.length > 0) {
                params.set("rpcs", rpcs.join(","));
            }

            const response = await fetch(`/api/health?${params.toString()}`);

            if (!response.ok) {
                throw new Error(`Health check failed: ${response.statusText}`);
            }

            const data: HealthCheckResponse = await response.json();

            logger.info(
                `Found ${data.apis.length} healthy APIs and ${data.rpcs.length} healthy RPCs (cached: ${data.cached}, age: ${data.age}s)`,
                { chainPath, healthyApis: data.apis, healthyRpcs: data.rpcs },
            );

            return { apis: data.apis, rpcs: data.rpcs };
        },
        enabled: !!((apis && apis.length > 0) || (rpcs && rpcs.length > 0)),
        staleTime: 30 * 1000, // 30 seconds
        gcTime: 10 * 60 * 1000, // 10 minutes
        refetchOnWindowFocus: false,
        retry: 2,
    });
}

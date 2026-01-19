import { useQuery } from "@tanstack/react-query";
import clientLogger from "@/lib/clientLogger";

function useFetchHealthyEndpoints(chainId: string) {
    return useQuery<{ apis: string[]; rpcs: string[] }>({
        queryKey: ["healthyApisAndRpcs", chainId],
        queryFn: async () => {
            const response = await fetch(`/api/health?chainPath=${chainId}`);
            const data: { apis: string[]; rpcs: string[] } = await response.json();
            return { apis: data.apis, rpcs: data.rpcs };
        },
        enabled: !!chainId,
        staleTime: 15 * 1000, // 15 seconds
        gcTime: 2 * 60 * 1000, // 2 minutes
        refetchOnWindowFocus: false,
        retryDelay: 1000, // 1 second
        retryOnMount: true,
        retry: 2,
    });
}

export function useGetRandomHealthyApi(chainId: string) {
    const { data } = useFetchHealthyEndpoints(chainId);
    return data?.apis[Math.floor(Math.random() * data.apis.length)];
}

export function useGetRandomHealthyRpc(chainId: string) {
    const { data } = useFetchHealthyEndpoints(chainId);
    return data?.rpcs[Math.floor(Math.random() * data.rpcs.length)];
}

/**
 * Non-hook version for imperative use (e.g., in async functions, tasks, etc.)
 * Fetches healthy endpoints directly without React hooks
 */
export async function getRandomHealthyApiImperative(chainId: string): Promise<string | null> {
    try {
        const response = await fetch(`/api/health?chainPath=${chainId}`);
        if (!response.ok) {
            clientLogger.error(`Health check returned status ${response.status} for chain ${chainId}`);
            return null;
        }
        const data: { apis: string[]; rpcs: string[] } = await response.json();
        if (!data.apis || data.apis.length === 0) {
            clientLogger.warn(`No healthy APIs found for chain ${chainId}`);
            return null;
        }
        const api = data.apis[Math.floor(Math.random() * data.apis.length)];
        if (!api) {
            clientLogger.error(`Selected API is undefined for chain ${chainId}`);
            return null;
        }
        return api;
    } catch (error) {
        clientLogger.error(`Error fetching healthy API for chain ${chainId}:`, error);
        return null;
    }
}

/**
 * Non-hook version for imperative use (e.g., in async functions, tasks, etc.)
 * Fetches healthy endpoints directly without React hooks
 */
export async function getRandomHealthyRpcImperative(chainId: string): Promise<string | null> {
    try {
        const response = await fetch(`/api/health?chainPath=${chainId}`);
        if (!response.ok) {
            clientLogger.error(`Health check returned status ${response.status} for chain ${chainId}`);
            return null;
        }
        const data: { apis: string[]; rpcs: string[] } = await response.json();
        if (!data.rpcs || data.rpcs.length === 0) {
            clientLogger.warn(`No healthy RPCs found for chain ${chainId}`);
            return null;
        }
        const rpc = data.rpcs[Math.floor(Math.random() * data.rpcs.length)];
        if (!rpc) {
            clientLogger.error(`Selected RPC is undefined for chain ${chainId}`);
            return null;
        }
        return rpc;
    } catch (error) {
        clientLogger.error(`Error fetching healthy RPC for chain ${chainId}:`, error);
        return null;
    }
}

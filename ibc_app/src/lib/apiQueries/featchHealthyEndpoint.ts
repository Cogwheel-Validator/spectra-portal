import { useQuery } from "@tanstack/react-query";

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

export function getRandomHealthyApi(chainId: string) {
    const { data } = useFetchHealthyEndpoints(chainId);
    return data?.apis[Math.floor(Math.random() * data.apis.length)];
}

export function getRandomHealthyRpc(chainId: string) {
    const { data } = useFetchHealthyEndpoints(chainId);
    return data?.rpcs[Math.floor(Math.random() * data.rpcs.length)];
}
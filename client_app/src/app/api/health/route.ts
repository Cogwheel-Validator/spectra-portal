import { type NextRequest, NextResponse } from "next/server";
import { LoadConfig } from "@/lib/config/config";
import logger from "@/lib/logger";

// Cache structure: chainPath -> { apis: string[], rpcs: string[], timestamp: number }
const healthCache = new Map<
    string,
    {
        apis: string[];
        rpcs: string[];
        timestamp: number;
    }
>();

// Load the config
const fullClientConfig = await LoadConfig();
// Cache TTL in milliseconds (120 seconds)
const CACHE_TTL = 120 * 1000;

// Health check timeout (3 seconds)
const HEALTH_CHECK_TIMEOUT = 3000;

// App URL
const appUrl: string = "https://ibc.thespectra.io";

// Get the chain paths ( chain paths = chainId ) with it's own RPCs and APIs endpoints
const chainPaths: Map<string, { apis: string[]; rpcs: string[] }> = new Map();
fullClientConfig.config.chains.forEach((chain) => {
    chainPaths.set(chain.id, {
        apis: fullClientConfig.getChainAPIs(chain.id),
        rpcs: fullClientConfig.getChainRPCs(chain.id),
    });
});

/**
 * Check if a CORS preflight request would succeed from the browser
 * This simulates what a browser would do before making the actual request
 */
async function checkCorsPreflightHealthy(url: string): Promise<boolean> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HEALTH_CHECK_TIMEOUT);

    try {
        const response = await fetch(url, {
            method: "OPTIONS",
            signal: controller.signal,
            headers: {
                Origin: appUrl,
                "Access-Control-Request-Method": "GET",
                "Access-Control-Request-Headers": "content-type",
            },
        });
        clearTimeout(timeoutId);

        // Check if CORS headers are present in response
        const corsHeader = response.headers.get("Access-Control-Allow-Origin");

        if (!corsHeader || (!corsHeader.includes("*") && !corsHeader.includes(appUrl))) {
            logger.warn(
                `CORS preflight failed: Missing or mismatched Access-Control-Allow-Origin header for ${url}`,
                {
                    url,
                    expectedOrigin: appUrl,
                    receivedOriginHeader: corsHeader,
                },
            );
            return false;
        }

        return true;
    } catch (error) {
        clearTimeout(timeoutId);
        logger.warn(
            `Preflight check failed for ${url}: ${error instanceof Error ? error.message : String(error)}`,
            {
                url,
                error: error instanceof Error ? error.message : String(error),
            },
        );
        return false;
    }
}

/**
 * Check if an RPC endpoint is healthy
 */
async function checkRpcHealth(rpc: string): Promise<[string, string, number]> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HEALTH_CHECK_TIMEOUT);

    try {
        // First check CORS preflight
        const corsPreflight = await checkCorsPreflightHealthy(`${rpc}/abci_info`);
        if (!corsPreflight) {
            return ["", "", 0];
        }

        // Now make the actual request with browser-like headers
        const response = await fetch(`${rpc}/abci_info`, {
            method: "GET",
            signal: controller.signal,
            headers: {
                "Content-Type": "application/json",
                Origin: appUrl,
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
                Referer: `${appUrl}/`,
                Accept: "application/json",
            },
        });
        clearTimeout(timeoutId);

        if (!response.ok) {
            logger.warn(`RPC returned non-OK status: ${response.status} for ${rpc}`, {
                rpc,
                status: response.status,
            });
            return ["", "", 0];
        }

        const data = await response.json();
        const version = data.result.response.version;
        const abciAppName = data.result.response.data;
        const lastBlockHeight = parseInt(data.result.response.last_block_height, 10);
        return [version, abciAppName, lastBlockHeight];
    } catch (error) {
        clearTimeout(timeoutId);

        if (error instanceof Error && error.name === "AbortError") {
            logger.warn(
                `timeout: RPC health check timed out for ${rpc} (${HEALTH_CHECK_TIMEOUT}ms)`,
                { rpc },
            );
        } else {
            logger.warn(
                `RPC health check failed for ${rpc}: ${error instanceof Error ? error.message : String(error)}`,
                {
                    rpc,
                    errorName: error instanceof Error ? error.name : "Unknown",
                    errorMessage: error instanceof Error ? error.message : String(error),
                },
            );
        }
        return ["", "", 0];
    }
}

async function checkApiHealth(api: string): Promise<[string, string, boolean]> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HEALTH_CHECK_TIMEOUT);

    try {
        // First check CORS preflight for both endpoints
        const corsPreflightNodeInfo = await checkCorsPreflightHealthy(
            `${api}/cosmos/base/tendermint/v1beta1/node_info`,
        );
        const corsPreflightSyncing = await checkCorsPreflightHealthy(
            `${api}/cosmos/base/tendermint/v1beta1/syncing`,
        );

        if (!corsPreflightNodeInfo || !corsPreflightSyncing) {
            clearTimeout(timeoutId);
            return ["", "", false];
        }

        // Now make the actual requests with browser-like headers
        const responses = await Promise.all([
            fetch(`${api}/cosmos/base/tendermint/v1beta1/node_info`, {
                method: "GET",
                signal: controller.signal,
                headers: {
                    "Content-Type": "application/json",
                    Origin: appUrl,
                    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
                    Referer: `${appUrl}/`,
                    Accept: "application/json",
                },
            }),
            fetch(`${api}/cosmos/base/tendermint/v1beta1/syncing`, {
                method: "GET",
                signal: controller.signal,
                headers: {
                    "Content-Type": "application/json",
                    Origin: appUrl,
                    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
                    Referer: `${appUrl}/`,
                    Accept: "application/json",
                },
            }),
        ]);
        clearTimeout(timeoutId);

        // Check if both responses are OK
        if (!responses[0].ok || !responses[1].ok) {
            logger.warn(`API returned non-OK status for ${api}`, {
                api,
                nodeInfoStatus: responses[0].status,
                syncingStatus: responses[1].status,
            });
            return ["", "", false];
        }

        const nodeInfo = await responses[0].json();
        const syncing = await responses[1].json();
        return [
            nodeInfo.default_node_info.version,
            nodeInfo.application_version.app_name,
            syncing.syncing,
        ];
    } catch (error) {
        clearTimeout(timeoutId);

        if (error instanceof Error && error.name === "AbortError") {
            logger.warn(
                `timeout: API health check timed out for ${api} (${HEALTH_CHECK_TIMEOUT}ms)`,
                { api },
            );
        } else {
            logger.warn(
                `API health check failed for ${api}: ${error instanceof Error ? error.message : String(error)}`,
                {
                    api,
                    errorName: error instanceof Error ? error.name : "Unknown",
                    errorMessage: error instanceof Error ? error.message : String(error),
                },
            );
        }
        return ["", "", false];
    }
}

/**
 * Check multiple endpoints in parallel
 */
async function checkEndpointsHealth(
    endpoints: string[],
    checking: "rpc" | "api",
): Promise<string[]> {
    const healthyEndpoints: string[] = new Array(endpoints.length);
    let highestBlockHeight = 0;
    await Promise.all(
        endpoints.map(async (endpoint, index) => {
            if (checking === "rpc") {
                const [version, abciAppName, lastBlockHeight] = await checkRpcHealth(endpoint);
                const acceptableHeightDiff = 100;
                if (lastBlockHeight > highestBlockHeight) {
                    highestBlockHeight = lastBlockHeight;
                }
                const isHealthy =
                    version !== "" &&
                    abciAppName !== "" &&
                    lastBlockHeight > 0 &&
                    Math.abs(highestBlockHeight - lastBlockHeight) <= acceptableHeightDiff;
                if (isHealthy) {
                    healthyEndpoints[index] = endpoint;
                }
            } else if (checking === "api") {
                const [version, abciAppName, syncing] = await checkApiHealth(endpoint);
                const isHealthy = version !== "" && abciAppName !== "" && syncing === false;
                if (isHealthy) {
                    healthyEndpoints[index] = endpoint;
                }
            }
        }),
    );

    // Filter out undefined values to only return actual healthy endpoints
    const filteredHealthyEndpoints = healthyEndpoints.filter((r) => r !== undefined);

    const healthyCount = filteredHealthyEndpoints.length;
    const endpointType = checking.toUpperCase();

    if (healthyCount === 0) {
        logger.error(
            `no healthy ${endpointType} endpoints found. All ${endpoints.length} endpoints failed health checks.`,
            {
                totalEndpoints: endpoints.length,
                healthyCount,
                endpoints,
            },
        );
    } else if (healthyCount < endpoints.length) {
        logger.warn(
            `some ${endpointType} endpoints are unhealthy (${healthyCount}/${endpoints.length} healthy)`,
            {
                healthyCount,
                totalEndpoints: endpoints.length,
                healthyEndpoints: filteredHealthyEndpoints,
            },
        );
    } else {
        logger.info(
            `all ${endpointType} endpoints are healthy (${healthyCount}/${endpoints.length})`,
            {
                healthyCount,
                totalEndpoints: endpoints.length,
            },
        );
    }

    return filteredHealthyEndpoints;
}

/**
 * GET /api/health?chainPath=X
 *
 * Query params:
 * - chainPath: identifier for the chain (for caching)
 */
export async function GET(request: NextRequest) {
    try {
        const searchParams = request.nextUrl.searchParams;
        const chainPath = searchParams.get("chainPath");
        if (!chainPath) {
            return NextResponse.json({ error: "Missing chainPath parameter" }, { status: 400 });
        }

        if (!chainPaths.has(chainPath)) {
            return NextResponse.json({ error: "Chain not found" }, { status: 404 });
        }

        // if cache is still valid, return the cached data
        const cached = healthCache.get(chainPath);
        const timeSinceCached = Date.now() - (cached?.timestamp ?? 0);
        if (cached && timeSinceCached < CACHE_TTL) {
            const age = `${Math.round(timeSinceCached / 1000)}s`;
            logger.info(`Returning cached data for chain ${chainPath}`, {
                chainPath,
                apis: cached.apis,
                rpcs: cached.rpcs,
                cached: true,
                age: age,
            });
            return NextResponse.json({
                chainPath,
                apis: cached.apis,
                rpcs: cached.rpcs,
                cached: true,
                age: age,
            });
        }

        // Perform health checks in parallel
        const [healthyApis, healthyRpcs] = await Promise.all([
            checkEndpointsHealth(chainPaths.get(chainPath)?.apis ?? [], "api"),
            checkEndpointsHealth(chainPaths.get(chainPath)?.rpcs ?? [], "rpc"),
        ]);

        // Update cache
        healthCache.set(chainPath, {
            apis: healthyApis,
            rpcs: healthyRpcs,
            timestamp: Date.now(),
        });

        return NextResponse.json({
            chainPath,
            apis: healthyApis,
            rpcs: healthyRpcs,
            cached: false,
            age: `${0}s`,
        });
    } catch (error) {
        console.error("Health check error:", error);
        return NextResponse.json({ error: "Failed to perform health checks" }, { status: 500 });
    }
}

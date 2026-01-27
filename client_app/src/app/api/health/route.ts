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

// Health check timeout (5 seconds)
const HEALTH_CHECK_TIMEOUT = 5000;

// App URL for CORS checks
const appUrl: string = "https://portal.thespectra.io";

// Get the chain paths ( chain paths = chainId ) with it's own RPCs and APIs endpoints
const chainPaths: Map<string, { apis: string[]; rpcs: string[] }> = new Map();
fullClientConfig.config.chains.forEach((chain) => {
    chainPaths.set(chain.id, {
        apis: fullClientConfig.getChainAPIs(chain.id),
        rpcs: fullClientConfig.getChainRPCs(chain.id),
    });
});

interface HealthCheckResult {
    endpoint: string;
    healthy: boolean;
    version?: string;
    abciAppName?: string;
    lastBlockHeight?: number;
    syncing?: boolean;
    error?: string;
}

/**
 * Check if a CORS preflight request would succeed from the browser
 * @param url - The URL to check
 * @returns True if the CORS preflight request would succeed, false otherwise
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

        const corsHeader = response.headers.get("Access-Control-Allow-Origin");
        return !!(corsHeader && (corsHeader.includes("*") || corsHeader.includes(appUrl)));
    } catch {
        clearTimeout(timeoutId);
        return false;
    }
}

/**
 * Check if an RPC endpoint is healthy
 * @param rpc - The RPC endpoint to check
 * @returns The health check result
 */
async function checkRpcHealth(rpc: string): Promise<HealthCheckResult> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HEALTH_CHECK_TIMEOUT);

    try {
        // Run CORS preflight and actual request in parallel
        const [corsPreflight, response] = await Promise.all([
            checkCorsPreflightHealthy(`${rpc}/abci_info`),
            fetch(`${rpc}/abci_info`, {
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

        if (!corsPreflight) {
            return { endpoint: rpc, healthy: false, error: "CORS preflight failed" };
        }

        if (!response.ok) {
            return { endpoint: rpc, healthy: false, error: `HTTP ${response.status}` };
        }

        const data = await response.json();
        const version = data.result.response.version;
        const abciAppName = data.result.response.data;
        const lastBlockHeight = parseInt(data.result.response.last_block_height, 10);

        return {
            endpoint: rpc,
            healthy: version !== "" && abciAppName !== "" && lastBlockHeight > 0,
            version,
            abciAppName,
            lastBlockHeight,
        };
    } catch (error) {
        clearTimeout(timeoutId);
        return {
            endpoint: rpc,
            healthy: false,
            error: error instanceof Error ? error.message : String(error),
        };
    }
}

/**
 * Check if an API endpoint is healthy
 * @param api - The API endpoint to check
 * @returns The health check result
 */
async function checkApiHealth(api: string): Promise<HealthCheckResult> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HEALTH_CHECK_TIMEOUT);

    try {
        const nodeInfoUrl = `${api}/cosmos/base/tendermint/v1beta1/node_info`;
        const syncingUrl = `${api}/cosmos/base/tendermint/v1beta1/syncing`;

        // Run CORS preflight checks and actual requests in parallel
        const [corsNodeInfo, corsSyncing, response1, response2] = await Promise.all([
            checkCorsPreflightHealthy(nodeInfoUrl),
            checkCorsPreflightHealthy(syncingUrl),
            fetch(nodeInfoUrl, {
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
            fetch(syncingUrl, {
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

        if (!corsNodeInfo || !corsSyncing) {
            return { endpoint: api, healthy: false, error: "CORS preflight failed" };
        }

        if (!response1.ok || !response2.ok) {
            return {
                endpoint: api,
                healthy: false,
                error: `HTTP ${response1.status}/${response2.status}`,
            };
        }

        const nodeInfo = await response1.json();
        const syncing = await response2.json();

        return {
            endpoint: api,
            healthy:
                nodeInfo.default_node_info.version !== "" &&
                nodeInfo.application_version.app_name !== "" &&
                syncing.syncing === false,
            version: nodeInfo.default_node_info.version,
            abciAppName: nodeInfo.application_version.app_name,
            syncing: syncing.syncing,
        };
    } catch (error) {
        clearTimeout(timeoutId);
        return {
            endpoint: api,
            healthy: false,
            error: error instanceof Error ? error.message : String(error),
        };
    }
}

/**
 * Check the health of multiple endpoints in parallel
 * @param endpoints - The endpoints to check
 * @param checking - The type of endpoints to check (rpc or api)
 * @returns The healthy endpoints
 */
async function checkEndpointsHealth(
    endpoints: string[],
    checking: "rpc" | "api",
): Promise<string[]> {
    // Run all health checks in parallel using Promise.all
    const results = await Promise.all(
        endpoints.map((endpoint) =>
            checking === "rpc" ? checkRpcHealth(endpoint) : checkApiHealth(endpoint),
        ),
    );

    // Find the highest block height (for RPC checks only)
    let highestBlockHeight = 0;
    if (checking === "rpc") {
        for (const result of results) {
            if (result.lastBlockHeight && result.lastBlockHeight > highestBlockHeight) {
                highestBlockHeight = result.lastBlockHeight;
            }
        }
    }

    // Filter healthy endpoints based on their criteria
    const healthyEndpoints: string[] = [];
    const acceptableHeightDiff = 100;

    for (const result of results) {
        if (checking === "rpc") {
            const isHealthy =
                result.healthy &&
                result.lastBlockHeight &&
                result.lastBlockHeight > 0 &&
                Math.abs(highestBlockHeight - result.lastBlockHeight) <= acceptableHeightDiff;
            if (isHealthy) {
                healthyEndpoints.push(result.endpoint);
            } else if (!result.healthy && result.error) {
                logger.warn(
                    {
                        rpc: result.endpoint,
                        error: result.error,
                    },
                    `RPC health check failed for ${result.endpoint}: ${result.error}`,
                );
            }
        } else if (checking === "api") {
            const isHealthy = result.healthy && result.syncing === false;
            if (isHealthy) {
                healthyEndpoints.push(result.endpoint);
            } else if (!result.healthy && result.error) {
                logger.warn(
                    {
                        api: result.endpoint,
                        error: result.error,
                    },
                    `API health check failed for ${result.endpoint}: ${result.error}`,
                );
            }
        }
    }

    const healthyCount = healthyEndpoints.length;
    const endpointType = checking.toUpperCase();

    if (healthyCount === 0) {
        logger.error(
            {
                totalEndpoints: endpoints.length,
                healthyCount,
                endpoints,
            },
            `no healthy ${endpointType} endpoints found. All ${endpoints.length} endpoints failed health checks.`,
        );
    } else if (healthyCount < endpoints.length) {
        logger.warn(
            {
                healthyCount,
                totalEndpoints: endpoints.length,
                healthyEndpoints,
            },
            `some ${endpointType} endpoints are unhealthy (${healthyCount}/${endpoints.length} healthy)`,
        );
    } else {
        logger.info(
            {
                healthyCount,
                totalEndpoints: endpoints.length,
            },
            `all ${endpointType} endpoints are healthy (${healthyCount}/${endpoints.length})`,
        );
    }

    return healthyEndpoints;
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
            logger.info(
                {
                    chainPath,
                    apis: cached.apis,
                    rpcs: cached.rpcs,
                    cached: true,
                    age: age,
                },
                `Returning cached data for chain ${chainPath}`,
            );
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

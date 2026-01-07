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

// Health check timeout (2 seconds)
const HEALTH_CHECK_TIMEOUT = 2000;

// Get the chain paths ( chain paths = chainId ) with it's own RPCs and APIs endpoints
const chainPaths: Map<string, { apis: string[]; rpcs: string[] }> = new Map();
fullClientConfig.config.chains.forEach((chain) => {
    chainPaths.set(chain.id, {
        apis: fullClientConfig.getChainAPIs(chain.id),
        rpcs: fullClientConfig.getChainRPCs(chain.id),
    });
});
/**
 * Check if an RPC endpoint is healthy
 */
async function checkRpcHealth(rpc: string): Promise<[string, string, number]> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HEALTH_CHECK_TIMEOUT);

    try {
        const response = await fetch(`${rpc}/abci_info`, {
            method: "GET",
            signal: controller.signal,
            headers: {
                "Content-Type": "application/json",
                Origin: "https://ibc.thespectra.io", // So if there is some cors blocking the endpoint
            },
        });
        clearTimeout(timeoutId);
        const data = await response.json();
        const version = data.result.response.version;
        const abciAppName = data.result.response.data;
        const lastBlockHeight = parseInt(data.result.response.last_block_height, 10);
        return [version, abciAppName, lastBlockHeight];
    } catch (error) {
        console.log(`Error when RPC checking ${rpc}: ${error}`);
        clearTimeout(timeoutId);
        return ["", "", 0];
    }
}

async function checkApiHealth(api: string): Promise<[string, string, boolean]> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HEALTH_CHECK_TIMEOUT);

    try {
        const responses = await Promise.all([
            fetch(`${api}/cosmos/base/tendermint/v1beta1/node_info`, {
                method: "GET",
                signal: controller.signal,
                headers: {
                    "Content-Type": "application/json",
                    Origin: "https://ibc.thespectra.io", // So if there is some cors blocking the endpoint
                },
            }),
            fetch(`${api}/cosmos/base/tendermint/v1beta1/syncing`, {
                method: "GET",
                signal: controller.signal,
                headers: {
                    "Content-Type": "application/json",
                    Origin: "https://ibc.thespectra.io", // So if there is some cors blocking the endpoint
                },
            }),
        ]);
        clearTimeout(timeoutId);
        const nodeInfo = await responses[0].json();
        const syncing = await responses[1].json();
        return [
            nodeInfo.default_node_info.version,
            nodeInfo.application_version.app_name,
            syncing.syncing,
        ];
    } catch (error) {
        console.log(`Error when API checking ${api}: ${error}`);
        clearTimeout(timeoutId);
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

    logger.info(
        `Checked ${endpoints.length} endpoints, ${healthyEndpoints.filter((r) => r !== undefined).length} healthy`,
        {
            endpoints,
            healthyEndpoints,
        },
    );

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

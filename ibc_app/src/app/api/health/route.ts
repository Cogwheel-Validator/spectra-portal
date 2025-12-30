import { type NextRequest, NextResponse } from "next/server";

// Cache structure: chainPath -> { apis: string[], rpcs: string[], timestamp: number }
const healthCache = new Map<
    string,
    {
        apis: string[];
        rpcs: string[];
        timestamp: number;
    }
>();

// Cache TTL in milliseconds (120 seconds)
const CACHE_TTL = 120 * 1000;

// Health check timeout (2 seconds)
const HEALTH_CHECK_TIMEOUT = 2000;

/**
 * Check if an API endpoint is healthy
 */
async function checkApiHealth(api: string): Promise<boolean> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HEALTH_CHECK_TIMEOUT);

    try {
        const response = await fetch(`${api}/cosmos/base/tendermint/v1beta1/node_info`, {
            method: "GET",
            signal: controller.signal,
            headers: {
                "Content-Type": "application/json",
                Origin: "https://ibc.thespectra.io", // So if there is some cors blocking the endpoint
            },
        });
        clearTimeout(timeoutId);
        return response.ok;
    } catch (error) {
        clearTimeout(timeoutId);
        console.log(`Error when API checking ${api}: ${error}`);
        return false;
    }
}

/**
 * Check if an RPC endpoint is healthy
 */
async function checkRpcHealth(rpc: string): Promise<boolean> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HEALTH_CHECK_TIMEOUT);

    try {
        const response = await fetch(`${rpc}/status`, {
            method: "GET",
            signal: controller.signal,
            headers: {
                "Content-Type": "application/json",
                Origin: "https://ibc.thespectra.io", // So if there is some cors blocking the endpoint
            },
        });
        clearTimeout(timeoutId);
        return response.ok;
    } catch (error) {
        console.log(`Error when RPC checking ${rpc}: ${error}`);
        clearTimeout(timeoutId);
        return false;
    }
}

/**
 * Check multiple endpoints in parallel
 */
async function checkEndpointsHealth(
    endpoints: string[],
    checker: (url: string) => Promise<boolean>,
): Promise<string[]> {
    const results = await Promise.all(
        endpoints.map(async (endpoint) => {
            const isHealthy = await checker(endpoint);
            return { endpoint, isHealthy };
        }),
    );

    return results.filter((r) => r.isHealthy).map((r) => r.endpoint);
}

/**
 * GET /api/health?chainPath=X&apis=url1,url2&rpcs=url3,url4
 *
 * Query params:
 * - chainPath: identifier for the chain (for caching)
 * - apis: comma-separated list of API endpoints
 * - rpcs: comma-separated list of RPC endpoints
 */
export async function GET(request: NextRequest) {
    try {
        const searchParams = request.nextUrl.searchParams;
        const chainPath = searchParams.get("chainPath");
        const apisParam = searchParams.get("apis");
        const rpcsParam = searchParams.get("rpcs");

        if (!chainPath) {
            return NextResponse.json({ error: "Missing chainPath parameter" }, { status: 400 });
        }

        const apis = apisParam ? apisParam.split(",").filter(Boolean) : [];
        const rpcs = rpcsParam ? rpcsParam.split(",").filter(Boolean) : [];

        if (apis.length === 0 && rpcs.length === 0) {
            return NextResponse.json(
                { error: "Must provide at least one API or RPC endpoint" },
                { status: 400 },
            );
        }

        // Check cache first
        const cached = healthCache.get(chainPath);
        const now = Date.now();

        if (cached && now - cached.timestamp < CACHE_TTL) {
            return NextResponse.json({
                chainPath,
                apis: cached.apis,
                rpcs: cached.rpcs,
                cached: true,
                age: Math.round((now - cached.timestamp) / 1000),
            });
        }

        // Perform health checks in parallel
        const [healthyApis, healthyRpcs] = await Promise.all([
            apis.length > 0 ? checkEndpointsHealth(apis, checkApiHealth) : Promise.resolve([]),
            rpcs.length > 0 ? checkEndpointsHealth(rpcs, checkRpcHealth) : Promise.resolve([]),
        ]);

        // Update cache
        healthCache.set(chainPath, {
            apis: healthyApis,
            rpcs: healthyRpcs,
            timestamp: now,
        });

        return NextResponse.json({
            chainPath,
            apis: healthyApis,
            rpcs: healthyRpcs,
            cached: false,
            age: 0,
        });
    } catch (error) {
        console.error("Health check error:", error);
        return NextResponse.json({ error: "Failed to perform health checks" }, { status: 500 });
    }
}

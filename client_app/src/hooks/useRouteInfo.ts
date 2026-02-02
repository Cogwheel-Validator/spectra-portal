import { useMemo } from "react";
import type { TransferMode } from "@/context/transferContext";
import { getRouteStepCount, routeSupportsPfm } from "@/hooks/usePathfinderQuery";

export interface RouteInfo {
    routeType: string;
    expectedOutput: string;
    priceImpact: number;
    priceImpactBps: number;
    priceImpactColor: string;
    stepCount: number;
}

export function useRouteInfo(pathfinderResponse: any, mode: TransferMode) {
    const supportsPfm = useMemo(() => {
        return pathfinderResponse ? routeSupportsPfm(pathfinderResponse) : false;
    }, [pathfinderResponse]);

    const supportsWasm = useMemo(() => {
        if (!pathfinderResponse?.success) return false;
        if (pathfinderResponse.route.case === "brokerSwap") {
            return pathfinderResponse.route.value.execution?.usesWasm ?? false;
        }
        return false;
    }, [pathfinderResponse]);

    const stepCount = useMemo(() => {
        return pathfinderResponse ? getRouteStepCount(pathfinderResponse, mode) : 0;
    }, [pathfinderResponse, mode]);

    const isDirectRoute = useMemo(() => {
        if (!pathfinderResponse?.success) return false;
        return pathfinderResponse.route.case === "direct";
    }, [pathfinderResponse]);

    const routeInfo = useMemo((): RouteInfo | null => {
        if (!pathfinderResponse?.success) return null;

        let routeType = "";
        let expectedOutput = "";
        let priceImpact = 0;
        let priceImpactBps = 0;
        let priceImpactColor = "";

        switch (pathfinderResponse.route.case) {
            case "direct":
                routeType = "Direct Transfer";
                expectedOutput = pathfinderResponse.route.value.transfer?.amount ?? "0";
                break;
            case "indirect": {
                routeType = "Multi-hop Transfer";
                const legs = pathfinderResponse.route.value.legs;
                expectedOutput = legs[legs.length - 1]?.amount ?? "0";
                break;
            }
            case "brokerSwap":
                routeType = "Swap & Transfer";
                expectedOutput = pathfinderResponse.route.value.swap?.amountOut ?? "0";
                priceImpact = Number.parseFloat(
                    pathfinderResponse.route.value.swap?.priceImpact ?? "0",
                );
                priceImpactBps = Math.round(priceImpact * 10000);

                if (priceImpactBps < -500) {
                    priceImpactColor = "#e11d48"; // rose-600
                } else if (priceImpactBps < -250) {
                    priceImpactColor = "#f87171"; // red-400
                } else if (priceImpactBps < -100) {
                    priceImpactColor = "#facc15"; // yellow-400
                } else {
                    priceImpactColor = "#4ade80"; // green-400
                }
                break;
        }

        return {
            routeType,
            expectedOutput,
            priceImpact,
            priceImpactBps,
            priceImpactColor,
            stepCount,
        };
    }, [pathfinderResponse, stepCount]);

    const intermediateChainIds = useMemo(() => {
        if (!pathfinderResponse?.success) return [];

        let chainPath: string[] = [];
        switch (pathfinderResponse.route.case) {
            case "indirect":
                chainPath = pathfinderResponse.route.value.path;
                break;
            case "brokerSwap":
                chainPath = pathfinderResponse.route.value.path;
                break;
            default:
                return [];
        }

        if (chainPath.length > 2) {
            return chainPath.slice(1, -1);
        }
        return [];
    }, [pathfinderResponse]);

    return {
        routeInfo,
        supportsPfm,
        supportsWasm,
        isDirectRoute,
        intermediateChainIds,
    };
}


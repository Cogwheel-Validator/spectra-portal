"use client";
import {
    type CallbackClient,
    type Client,
    createCallbackClient,
    createClient,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { PathfinderService } from "@/lib/generated/pathfinder/pathfinder_route_pb";

/**
 * Creates a client for the Pathfinder service from which you can call the Pathfinder service methods
 * @param callback - if true, return a callback client, otherwise return a regular client
 * @returns a client for the Pathfinder service
 */
export const pathfinderClient = (
    callback: boolean,
): Client<typeof PathfinderService> | CallbackClient<typeof PathfinderService> => {
    const baseUrl = process.env.NEXT_PUBLIC_PATHFINDER_RPC_URL || "http://localhost:8080";

    const transport = createConnectTransport({
        baseUrl: baseUrl,
    });

    if (callback) {
        return createCallbackClient(PathfinderService, transport);
    } else {
        return createClient(PathfinderService, transport);
    }
};

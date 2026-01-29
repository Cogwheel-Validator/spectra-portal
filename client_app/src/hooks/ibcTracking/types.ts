export interface PacketData {
    packetDataHex: string;
    // packet timeout is technically a Unix timestamp but it is stored as a string
    packetTimeout: string;
    packetSequence: string;
    packetSrcPort: string;
    packetSrcChannel: string;
    packetDstPort: string;
    packetDstChannel: string;
    packetChannelOrdering: string;
    packetConnectionId: string;
}

export interface IbcTrackingResult {
    success: boolean;
    txHash?: string;
    error?: string;
}

export interface TrackingOptions {
    maxAttempts?: number; // Maximum polling attempts (default: 60)
    pollInterval?: number; // Milliseconds between polls (default: 10000 = 10s)
    timeout?: number; // Total timeout in ms (default: 10 minutes)
    onProgress?: (attempt: number, maxAttempts: number) => void;
}

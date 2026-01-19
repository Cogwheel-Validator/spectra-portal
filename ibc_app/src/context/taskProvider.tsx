"use client";

import type { EncodeObject } from "@cosmjs/proto-signing";
import {
    createContext,
    type ReactNode,
    useCallback,
    useContext,
    useEffect,
    useMemo,
    useRef,
    useState,
} from "react";
import type { ClientChain, ClientConfig } from "@/components/modules/tomlTypes";
import useIbcTracking from "@/hooks/useIbcTracking";
import clientLogger from "@/lib/clientLogger";
import type { SwapAmountInRoute } from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/swap_route";
import type {
    BrokerSwapRoute,
    FindPathResponse,
} from "@/lib/generated/pathfinder/pathfinder_route_pb";
import {
    AddTransactionToDb,
    ConnectToDb,
    type TransactionRecord,
    UpdateTransactionStatus,
} from "@/lib/indexDb/dbManager";
import {
    extractChainPath,
    generateStepsFromResponse,
    type TransferMode,
    type TransferStep,
    useTransfer,
} from "./transferContext";
import { useWallet } from "./walletContext";

// ============================================================================
// Types
// ============================================================================

export const TaskTypes = {
    // IBC transfers
    IBC_TRANSFER: "ibc-basic-transfer",
    IBC_HOOKS_TRANSFER: "ibc-hooks-transfer",
    IBC_PFM_TRANSFER: "ibc-pfm-transfer",
    // Osmosis swaps
    OSMO_SWAP: "osmosis-swap",
    OSMO_SPLIT_ROUTE_SWAP: "osmosis-split-route-swap",
    // Indexed DB related tasks
    INSERT_TRANSACTION: "insert-transaction",
    UPDATE_TRANSACTION: "update-transaction",
} as const;

export type TaskType = (typeof TaskTypes)[keyof typeof TaskTypes];

/**
 * Execution result from a task
 */
export interface TaskExecutionResult {
    success: boolean;
    txHash?: string;
    error?: string;
    isRetryable?: boolean;
}

/**
 * Task context interface
 */
interface TaskContextType {
    // Database connection
    db: IDBDatabase | null;
    isDbReady: boolean;

    // Execution methods
    executeTransfer: (
        response: FindPathResponse,
        mode: TransferMode,
        config: ClientConfig,
    ) => Promise<void>;
    executeStep: (step: TransferStep, config: ClientConfig) => Promise<TaskExecutionResult>;
    retryStep: (stepIndex: number, config: ClientConfig) => Promise<TaskExecutionResult>;

    // State
    isExecuting: boolean;
}

const TaskContext = createContext<TaskContextType | undefined>(undefined);

export const useTaskProvider = () => {
    const context = useContext(TaskContext);
    if (!context) {
        throw new Error("useTaskProvider must be used within TaskProvider");
    }
    return context;
};

// ============================================================================
// Provider
// ============================================================================

export const TaskProvider = ({ children }: { children: ReactNode }) => {
    const [db, setDb] = useState<IDBDatabase | null>(null);
    const [isDbReady, setIsDbReady] = useState(false);
    const [isExecuting, setIsExecuting] = useState(false);
    const executionLock = useRef(false);

    const { sendTransaction, getAddress } = useWallet();
    const transfer = useTransfer();
    const { trackIbcTransfer } = useIbcTracking();

    // Initialize database connection
    useEffect(() => {
        let mounted = true;

        const initDb = async () => {
            try {
                const database = await ConnectToDb();
                if (mounted) {
                    setDb(database);
                    setIsDbReady(true);
                }
            } catch (error) {
                clientLogger.error("Failed to connect to IndexedDB:", error);
            }
        };

        initDb();

        return () => {
            mounted = false;
        };
    }, []);

    /**
     * Creates an IBC transfer message
     */
    const createIbcTransferMessage = useCallback(
        (
            sender: string,
            receiver: string,
            sourceChannel: string,
            sourcePort: string,
            token: { amount: string; denom: string },
            memo: string = "",
            timeoutMinutes: number = 10,
        ): EncodeObject => {
            const timeoutTimestamp = `${(Date.now() + timeoutMinutes * 60 * 1000).toString()}000000`;

            return {
                typeUrl: "/ibc.applications.transfer.v1.MsgTransfer",
                value: {
                    sourcePort,
                    sourceChannel,
                    token,
                    sender,
                    receiver,
                    timeoutHeight: {
                        revisionHeight: "0",
                        revisionNumber: "0",
                    },
                    timeoutTimestamp,
                    memo,
                },
            };
        },
        [],
    );

    /**
     * Creates an Osmosis swap message
     */
    const createSwapMessage = useCallback(
        (
            sender: string,
            tokenIn: { amount: string; denom: string },
            routes: SwapAmountInRoute[],
            tokenOutMinAmount: string,
        ): EncodeObject => {
            return {
                typeUrl: "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn",
                value: {
                    sender,
                    tokenIn,
                    routes,
                    tokenOutMinAmount,
                },
            };
        },
        [],
    );

    /**
     * Execute a single transfer step
     */
    const executeStep = useCallback(
        async (step: TransferStep, config: ClientConfig): Promise<TaskExecutionResult> => {
            const chainConfig = config.chains.find((c) => c.id === step.fromChain);
            if (!chainConfig) {
                return { success: false, error: `Chain config not found for ${step.fromChain}` };
            }

            const senderAddress = getAddress(step.fromChain);
            if (!senderAddress) {
                return {
                    success: false,
                    error: `Not connected to ${step.fromChain}`,
                    isRetryable: true,
                };
            }

            try {
                let message: EncodeObject;

                switch (step.type) {
                    case "ibc_transfer":
                    case "pfm_transfer": {
                        const receiverAddress =
                            getAddress(step.toChain) || transfer.state.receiverAddress;
                        if (!receiverAddress) {
                            return {
                                success: false,
                                error: `Receiver address not available for ${step.toChain}`,
                                isRetryable: true,
                            };
                        }

                        message = createIbcTransferMessage(
                            senderAddress,
                            receiverAddress,
                            step.metadata?.channel || "",
                            step.metadata?.port || "transfer",
                            {
                                amount: step.metadata?.amount || "0",
                                denom: step.metadata?.denom || "",
                            },
                            step.metadata?.memo || "",
                        );
                        break;
                    }

                    case "wasm_execution": {
                        // For WASM execution, the receiver is the contract address
                        const execution = (
                            transfer.state.pathfinderResponse?.route.value as BrokerSwapRoute
                        )?.execution;
                        if (!execution) {
                            return { success: false, error: "Execution data not found" };
                        }

                        // Get the first leg info for the IBC transfer
                        const brokerRoute = transfer.state.pathfinderResponse?.route
                            .value as BrokerSwapRoute;
                        const firstLeg = brokerRoute?.inboundLeg;

                        if (!firstLeg) {
                            // If no inbound leg, we're starting from the broker chain
                            // This means we need to execute the swap directly
                            return {
                                success: false,
                                error: "WASM execution from broker chain not yet implemented",
                            };
                        }

                        message = createIbcTransferMessage(
                            senderAddress,
                            execution.ibcReceiver,
                            firstLeg.channel,
                            firstLeg.port,
                            {
                                amount: firstLeg.amount,
                                denom: firstLeg.token?.chainDenom || "",
                            },
                            execution.memo,
                        );
                        break;
                    }

                    case "swap": {
                        // Osmosis swap
                        const brokerRoute = transfer.state.pathfinderResponse?.route
                            .value as BrokerSwapRoute;
                        const swapData = brokerRoute?.swap;
                        const osmosisRouteData = swapData?.routeData;

                        if (!swapData || osmosisRouteData?.case !== "osmosisRouteData") {
                            return { success: false, error: "Swap route data not found" };
                        }

                        const routes: SwapAmountInRoute[] = osmosisRouteData.value.routes.flatMap(
                            (route) =>
                                route.pools.map((pool) => ({
                                    poolId: Number(pool.id),
                                    tokenOutDenom: pool.tokenOutDenom,
                                })),
                        );

                        message = createSwapMessage(
                            senderAddress,
                            {
                                amount: swapData.amountIn,
                                denom: swapData.tokenIn?.chainDenom || "",
                            },
                            routes,
                            swapData.amountOut, // Use expected output as minimum
                        );
                        break;
                    }

                    default:
                        return { success: false, error: `Unknown step type: ${step.type}` };
                }

                const result = await sendTransaction(step.fromChain, [message], {
                    memo: "Spectra IBC Transfer",
                    chainConfig,
                });

                if (result.code === 0) {
                    return { success: true, txHash: result.transactionHash };
                } else {
                    return {
                        success: false,
                        error: `Transaction failed with code ${result.code}`,
                        txHash: result.transactionHash,
                        isRetryable: true,
                    };
                }
            } catch (error) {
                const errorMessage = error instanceof Error ? error.message : "Unknown error";
                const isRetryable =
                    errorMessage.toLowerCase().includes("out of gas") ||
                    errorMessage.toLowerCase().includes("insufficient") ||
                    errorMessage.toLowerCase().includes("timeout") ||
                    errorMessage.toLowerCase().includes("rejected");

                return { success: false, error: errorMessage, isRetryable };
            }
        },
        [
            getAddress,
            sendTransaction,
            transfer.state.pathfinderResponse,
            transfer.state.receiverAddress,
            createIbcTransferMessage,
            createSwapMessage,
        ],
    );

    /**
     * Execute the complete transfer flow
     */
    const executeTransfer = useCallback(
        async (
            response: FindPathResponse,
            mode: TransferMode,
            config: ClientConfig,
        ): Promise<void> => {
            if (!db || !response.success) {
                transfer.failTransfer("Invalid transfer state");
                return;
            }

            if (executionLock.current) {
                clientLogger.warn("Transfer already executing, ignoring duplicate call");
                return;
            }

            executionLock.current = true;
            setIsExecuting(true);

            try {
                // Generate steps from the pathfinder response
                const steps = generateStepsFromResponse(response, mode);
                const chainPath = extractChainPath(response);

                if (steps.length === 0) {
                    transfer.failTransfer("No steps generated from route");
                    return;
                }

                // Create transaction record in IndexedDB
                const transactionRecord: Omit<TransactionRecord, "id"> = {
                    timestamp: new Date().toISOString(),
                    fromChainId: transfer.state.fromChainId,
                    fromChainAddress: transfer.state.senderAddress,
                    toChainId: transfer.state.toChainId,
                    toChainAddress: transfer.state.receiverAddress,
                    tokenIn: transfer.state.fromToken?.denom || "",
                    tokenOut:
                        transfer.state.toToken?.denom || transfer.state.fromToken?.denom || "",
                    amountIn: transfer.state.amount,
                    amountOut: getExpectedOutput(response),
                    typeOfTransfer: mode,
                    status: "in-progress",
                    totalSteps: steps.length,
                    currentStep: 0,
                    trajectory: chainPath.length > 2 ? chainPath.slice(1, -1) : null,
                    error: null,
                };

                const dbRecordId = await AddTransactionToDb(db, transactionRecord);

                // Start execution with generated steps
                transfer.startExecuting(steps, dbRecordId, chainPath);

                // Execute each step sequentially
                for (let i = 0; i < steps.length; i++) {
                    const step = steps[i];
                    transfer.updateStep(i, { status: "signing" });

                    const result = await executeStep(step, config);

                    if (result.success && result.txHash) {
                        // Check if this is an IBC transfer that needs tracking
                        const isIbcStep =
                            step.type === "ibc_transfer" ||
                            step.type === "pfm_transfer" ||
                            step.type === "wasm_execution";

                        if (isIbcStep && i < steps.length - 1) {
                            // Update to "confirming" status while we track
                            transfer.updateStep(i, {
                                status: "confirming",
                                txHash: result.txHash,
                            });

                            // Get destination chain config for tracking
                            const destChain = config.chains.find((c) => c.id === step.toChain) as
                                | ClientChain
                                | undefined;

                            if (destChain) {
                                // Track the IBC transfer on destination chain
                                const trackingResult = await trackIbcTransfer(
                                    step.fromChain,
                                    result.txHash,
                                    destChain,
                                    {
                                        maxAttempts: 60, // 60 attempts
                                        pollInterval: 10000, // 10 seconds between polls
                                        timeout: 10 * 60 * 1000, // 10 minute total timeout
                                        onProgress: (attempt, max) => {
                                            // Optional: could update UI with progress
                                            clientLogger.info(
                                                `Tracking IBC transfer: attempt ${attempt}/${max}`,
                                            );
                                        },
                                    },
                                );

                                clientLogger.info("trackingResult", trackingResult);

                                if (!trackingResult.success) {
                                    // Tracking failed - this doesn't necessarily mean the transfer failed
                                    // It could just mean we couldn't confirm it in time
                                    // Mark as "completed" but log the warning
                                    clientLogger.warn(
                                        `IBC tracking inconclusive: ${trackingResult.error}`,
                                    );
                                }
                            }
                        }

                        // Mark step as completed
                        transfer.updateStep(i, {
                            status: "completed",
                            txHash: result.txHash,
                        });

                        // Update database
                        await UpdateTransactionStatus(db, {
                            id: dbRecordId,
                            currentStep: i + 1,
                        });

                        // If not the last step, advance
                        if (i < steps.length - 1) {
                            transfer.advanceStep();
                        }
                    } else {
                        transfer.updateStep(i, {
                            status: "failed",
                            error: result.error,
                        });

                        await UpdateTransactionStatus(db, {
                            id: dbRecordId,
                            status: "failed",
                            error: result.error || "Unknown error",
                        });

                        transfer.failTransfer(result.error || "Step execution failed");
                        setIsExecuting(false);
                        return;
                    }
                }

                // All steps completed
                await UpdateTransactionStatus(db, {
                    id: dbRecordId,
                    status: "success",
                    currentStep: steps.length,
                });

                transfer.completeTransfer();
            } catch (error) {
                const errorMessage = error instanceof Error ? error.message : "Unknown error";
                transfer.failTransfer(errorMessage);
            } finally {
                executionLock.current = false;
                setIsExecuting(false);
            }
        },
        [db, transfer, executeStep, trackIbcTransfer],
    );

    /**
     * Retry a failed step
     */
    const retryStep = useCallback(
        async (stepIndex: number, config: ClientConfig): Promise<TaskExecutionResult> => {
            const step = transfer.state.steps[stepIndex];
            if (!step) {
                return { success: false, error: "Step not found" };
            }

            setIsExecuting(true);
            transfer.updateStep(stepIndex, { status: "signing", error: undefined });

            const result = await executeStep(step, config);

            if (result.success) {
                transfer.updateStep(stepIndex, {
                    status: "completed",
                    txHash: result.txHash,
                });

                if (db && transfer.state.dbRecordId) {
                    await UpdateTransactionStatus(db, {
                        id: transfer.state.dbRecordId,
                        currentStep: stepIndex + 1,
                        error: null,
                    });
                }
            } else {
                transfer.updateStep(stepIndex, {
                    status: "failed",
                    error: result.error,
                });
            }

            setIsExecuting(false);
            return result;
        },
        [db, transfer, executeStep],
    );

    const contextValue = useMemo(
        () => ({
            db,
            isDbReady,
            executeTransfer,
            executeStep,
            retryStep,
            isExecuting,
        }),
        [db, isDbReady, executeTransfer, executeStep, retryStep, isExecuting],
    );

    return <TaskContext.Provider value={contextValue}>{children}</TaskContext.Provider>;
};

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Get expected output amount from pathfinder response
 */
function getExpectedOutput(response: FindPathResponse): string {
    if (!response.success) return "0";

    switch (response.route.case) {
        case "direct":
            return response.route.value.transfer?.amount || "0";
        case "indirect": {
            const legs = response.route.value.legs;
            return legs[legs.length - 1]?.amount || "0";
        }
        case "brokerSwap": {
            const route = response.route.value;
            if (route.outboundLegs.length > 0) {
                return route.outboundLegs[route.outboundLegs.length - 1].amount;
            }
            return route.swap?.amountOut || "0";
        }
        default:
            return "0";
    }
}
